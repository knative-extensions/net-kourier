/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package git

import "fmt"

// Info saves information that can be used to interact with GitHub.
type Info struct {
	Org      string
	Repo     string
	Head     string // PR head branch
	Base     string // PR base branch
	UserID   string // Github User ID of PR creator
	UserName string // User display name for Git commit
	Email    string // User email address for Git commit
}

// GetHeadRef returns the HeadRef with the given Git Info.
// HeadRef is in the form of "user:head", i.e. "github_user:branch_foo"
func (gi *Info) GetHeadRef() string {
	return fmt.Sprintf("%s:%s", gi.UserID, gi.Head)
}
