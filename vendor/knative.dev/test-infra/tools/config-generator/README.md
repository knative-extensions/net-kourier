# config-generator

config-generator is a tool that takes a meta config file (e.g.
../../config/prow/config_knative.yaml) and [templates](./templates) as input,
and generates configuration files for Prow and testgrid.

## Notice

As Knative evolves and more and more Prow jobs are required, this tool has
become clumsy and hard to maintain. There have been some initial discussions to
replace it with a more generic solution, but no clear outcome yet. If you have
any ideas on it, please join the discussion in Knative Productivity Slack
channel.
