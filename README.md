# create-pr-from-jira

```bash
Usage of ghpr:
  -base string
        The base branch for the pull request. This is optional and used as an override. The default is determined by using git symbolic-ref refs/remotes/origin/HEAD.
  -issue string
        The JIRA issue key (e.g., PROJECT-123)
```

| Environment Variable | Description |
|----------------------|-------------|
| GITHUB_TOKEN         | The GitHub token to use for authentication. |
| JIRA_API_TOKEN       | The JIRA token to use for authentication. |
| JIRA_EMAIL           | The JIRA user to use for authentication. |
| JIRA_BASE_URL        | The JIRA URL for the API. |
