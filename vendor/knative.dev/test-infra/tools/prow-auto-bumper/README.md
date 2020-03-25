# prow-auto-bumper

prow-auto-bumper is a tool that identifies recent stable Prow version in
k8s/test-infra repo, updates knative/test-infra to use this new version, and
creates Pull Request.

## Basic Usage

Flags for this tool are:

- `--github-account` [Required] specifies the path of file containing Github
  token for Github API calls.
- `--git-userid` [Required] specifies the Github ID of user for hosting fork,
  i.e. Github ID of bot.
- `--git-username` [Optional] specifies the username to use on the git commit.
  Requires --git-email.
- `--git-email` [Optional] specifies the email to use on the git commit.
  Requires --git-username.
- `--dry-run` [Optional] enables dry-run mode.
