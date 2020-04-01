# prow-config-updater

prow-config-updater is a tool that updates Prow configs with the files committed
in the latest pull request. Depends on which files are changed in the pull
request, it will either update production Prow directly, or update staging Prow
first and roll out the changes to production Prow after tests pass.

## Basic Usage

Flags for this tool are:

- `--github-token-file` [Required] specifies the path of file containing Github
  token for most of important interactions with Github.
- `--git-userid` [Required] specifies the Github ID of user for hosting fork,
  i.e. Github ID of bot.
- `--git-username` [Optional] specifies the username to use on the git commit.
  Requires --git-email.
- `--git-email` [Optional] specifies the email to use on the git commit.
  Requires --git-username.
- `--comment-github-token-file` [Required] specifies the path of file containing
  Github token for adding comments on the pull requests.
- `--dry-run` [Optional] enables dry-run mode.
