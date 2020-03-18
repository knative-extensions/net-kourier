/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"regexp"
	"testing"
)

const (
	sampleYaml = `spec:
  - error-pattern: 'Something went wrong: starting e2e cluster: error creating cluster'
    hint: 'Check gcp status'
    alerts:
      - job-name-regex: '^pull.*'
        occurrences: 2
        jobs-affected: 1
        prs-affected: 2
        period: 1440 # 1440 minutes = 24 hours
      - job-name-regex: '.*'
        occurrences: 5
        jobs-affected: 2
        prs-affected: 1 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 1440

  - error-pattern: 'sample*error2'
    hint: 'hint_for_pattern_2'
    alerts:
      - job-name-regex: '^pull.*'
        occurrences: 20
        jobs-affected: 10
        prs-affected: 20
        period: 60 # 1440 minutes = 24 hours
      - job-name-regex: '.*'
        occurrences: 50
        jobs-affected: 20
        prs-affected: 10 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 60`
)

var sampleConfig = Config{
	Spec: []patternSpec{
		{
			ErrorPattern: "Something went wrong: starting e2e cluster: error creating cluster",
			Hint:         "Check gcp status",
			Alerts: []alertCondition{
				{"^pull.*",
					2,
					1,
					2,
					1440},
				{".*",
					5,
					2,
					1,
					1440},
			},
		},

		{
			"sample*error2",
			"hint_for_pattern_2",
			[]alertCondition{
				{"^pull.*",
					20,
					10,
					20,
					60,
				},
				{".*",
					50,
					20,
					10,
					60,
				},
			},
		},
	},
}

func TestCollectErrorPatterns(t *testing.T) {
	conf, err := newConfig([]byte(sampleYaml))
	if err != nil {
		t.Errorf("cannot parse yaml: %v", err)
	}

	type args struct {
		f Config
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "CollectErrorPatterns",
			args: args{
				f: *conf,
			},
			want: []string{
				"Something went wrong: starting e2e cluster: error creating cluster",
				"sample*error2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.args.f.CollectErrorPatterns(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Collected Error Patterns, got: %v, want: %v", got, tt.want)
			}
		})
	}
}

func TestConfig_Select(t *testing.T) {
	type fields struct {
		Spec []patternSpec
	}
	type args struct {
		pattern string
		jobName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    SelectedConfig
		wantErr bool
	}{
		{
			name: "wild card pull job name pattern test",
			fields: fields{
				Spec: sampleConfig.Spec,
			},
			args: args{
				pattern: "sample*error2",
				jobName: "pull-is-me",
			},
			want: SelectedConfig{
				Hint:         "hint_for_pattern_2",
				Occurrences:  20,
				JobsAffected: 10,
				PrsAffected:  20,
				Period:       60,
			},
		},

		{
			name: "wild card non-pull job name pattern test",
			fields: fields{
				Spec: sampleConfig.Spec,
			},
			args: args{
				pattern: "sample*error2",
				jobName: "not-pull-me",
			},
			want: SelectedConfig{
				Hint:         "hint_for_pattern_2",
				Occurrences:  50,
				JobsAffected: 20,
				PrsAffected:  10,
				Period:       60,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Config{
				Spec: tt.fields.Spec,
			}
			got, err := f.Select(tt.args.pattern, tt.args.jobName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Config Selector returned error.")
				return
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Config selected, got : %v, want: %v", got, tt.want)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	type args struct {
		text []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *Config
		wantErr bool
	}{
		{
			name: "newConfig",
			args: args{
				text: []byte(sampleYaml),
			},
			want:    &sampleConfig,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newConfig(tt.args.text)
			if (err != nil) != tt.wantErr {
				t.Errorf("config construction failed")
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("config constructed,  got: %v, want: %v", got, tt.want)
			}
		})
	}
}

// validate checks if the given yaml content meets our requirement for a monitoring config file
func validate(text []byte) error {
	config, err := newConfig(text)
	if err != nil {
		return err
	}
	patterns := config.GetAllPatterns()

	if _, badPatterns := CompilePatterns(patterns); len(badPatterns) > 0 {
		return fmt.Errorf("bad patterns found: %v", badPatterns)
	}
	return nil
}

const yamlPath = "config.yaml"

func getConfigYaml() []byte {
	text, err := ioutil.ReadFile(yamlPath)
	if err != nil {
		log.Fatalf("cannot read file [%s]: %v", yamlPath, err)
		return nil
	}
	return text
}

func TestCompilePatterns(t *testing.T) {
	type args struct {
		patterns []string
	}
	tests := []struct {
		name              string
		args              args
		wantedRegexps     []regexp.Regexp
		wantedBadPatterns []string
	}{
		{
			name: "compile patterns",
			args: args{
				[]string{
					"Something went wrong: starting e2e cluster: error creating cluster",
					"sample*error2",
					"[0",
					"Something went wrong:.*\n",
				},
			},
			wantedRegexps: []regexp.Regexp{
				*regexp.MustCompile("Something went wrong: starting e2e cluster: error creating cluster"),
				*regexp.MustCompile("sample*error2"),
				*regexp.MustCompile("Something went wrong:.*\n"),
			},
			wantedBadPatterns: []string{
				"[0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiledPatterns, badPatterns := CompilePatterns(tt.args.patterns)
			if !reflect.DeepEqual(compiledPatterns, tt.wantedRegexps) {
				t.Errorf("all compiled patterns: compiledPatterns = %v, want %v", compiledPatterns, tt.wantedRegexps)
			}
			if !reflect.DeepEqual(badPatterns, tt.wantedBadPatterns) {
				t.Errorf("all bad patterns: compiledPatterns = %v, want %v", badPatterns, tt.wantedBadPatterns)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	type args struct {
		text []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			// this test is to test the validity of the actual config yaml.
			name: "actual config yaml",
			args: args{
				text: getConfigYaml(),
			},
			wantErr: false,
		},
		{
			name: "valid config yaml",
			args: args{
				text: []byte(`spec:
  - error-pattern: 'Something went wrong: starting e2e cluster: error creating cluster'
    hint: 'Check gcp status'
    alerts:
      - job-name-regex: 'pull.*'
        occurrences: 2
        jobs-affected: 1
        prs-affected: 2
        period: 1440 # 1440 minutes = 24 hours
      - job-name-regex: '.*'
        occurrences: 5
        jobs-affected: 2
        prs-affected: 1 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 1440

  - error-pattern: 'sample*error2'
    hint: 'hint_for_pattern_2'
    alerts:
      - job-name-regex: 'pull.*'
        occurrences: 20
        jobs-affected: 10
        prs-affected: 20
        period: 60 # 1440 minutes = 24 hours
      - job-name-regex: '.*'
        occurrences: 50
        jobs-affected: 20
        prs-affected: 10 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 60`),
			},
			wantErr: false,
		},
		{
			name: "bad yaml - bad error pattern",
			args: args{
				text: []byte(`spec:
  - error-pattern: 'Something went wrong: starting e2e cluster: error creating cluster'
    hint: 'Check gcp status'
    alerts:
      - job-name-regex: '^pull.*'
        occurrences: 2
        jobs-affected: 1
        prs-affected: 2
        period: 1440 # 1440 minutes = 24 hours
      - job-name-regex: '.*'
        occurrences: 5
        jobs-affected: 2
        prs-affected: 1 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 1440

  - error-pattern: '[3'
    hint: 'hint_for_pattern_2'
    alerts:
      - job-name-regex: '^pull.*'
        occurrences: 20
        jobs-affected: 10
        prs-affected: 20
        period: 60 # 1440 minutes = 24 hours
      - job-name-regex: '.*'
        occurrences: 50
        jobs-affected: 20
        prs-affected: 10 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 60`),
			},
			wantErr: true,
		},
		{
			name: "bad yaml - bad job name pattern",
			args: args{
				text: []byte(`spec:
  - error-pattern: 'Something went wrong: starting e2e cluster: error creating cluster'
    hint: 'Check gcp status'
    alerts:
      - job-name-regex: '^pull.*'
        occurrences: 2
        jobs-affected: 1
        prs-affected: 2
        period: 1440 # 1440 minutes = 24 hours
      - job-name-regex: '.*'
        occurrences: 5
        jobs-affected: 2
        prs-affected: 1 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 1440

  - error-pattern: '3'
    hint: 'hint_for_pattern_2'
    alerts:
      - job-name-regex: '^pull.*'
        occurrences: 20
        jobs-affected: 10
        prs-affected: 20
        period: 60 # 1440 minutes = 24 hours
      - job-name-regex: '[5'
        occurrences: 50
        jobs-affected: 20
        prs-affected: 10 # for non-pull jobs, we don't care about the number of prs affected, so we set the number to 1, which will basically make this particular condition always true
        period: 60`),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.args.text); (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
