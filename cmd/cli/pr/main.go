package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type JIRAIssue struct {
	Fields struct {
		Summary string `json:"summary"`
	} `json:"fields"`
}

type PullRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft"`
}

type PullRequestResponse struct {
	URL string `json:"url"`
}

var (
	jiraIssueKey = flag.String("issue", "", "The JIRA issue key (e.g., PROJECT-123)")
	baseBranch   = flag.String("base", "", "The base branch for the pull request")
)

func ptr(s string) *string {
	return &s
}

func getGitHubRepoDetails() (*string, *string, error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch remote origin URL: %w", err)
	}

	url := strings.TrimSpace(string(output))
	var owner, repo string

	if strings.HasPrefix(url, "https://") {
		parts := strings.Split(strings.TrimSuffix(url, ".git"), "/")
		if len(parts) >= 2 {
			owner = parts[len(parts)-2]
			repo = parts[len(parts)-1]
		}
	} else if strings.HasPrefix(url, "git@") {
		parts := strings.Split(strings.TrimSuffix(url, ".git"), ":")
		if len(parts) == 2 {
			subParts := strings.Split(parts[1], "/")
			if len(subParts) == 2 {
				owner = subParts[0]
				repo = subParts[1]
			}
		}
	}

	if owner == "" || repo == "" {
		return nil, nil, fmt.Errorf("could not parse owner and repo from URL: %s", url)
	}

	return &owner, &repo, nil
}

func getCurrentBranch() (*string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	return ptr(strings.TrimSpace(string(output))), nil
}

func getDefaultBranch() (*string, error) {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get default branch: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "/")
	if len(parts) > 0 {
		return &parts[len(parts)-1], nil
	}

	return nil, fmt.Errorf("could not parse default branch from output: %s", string(output))
}

func getJiraIssueTitle(jiraBaseURL, jiraEmail, jiraAPIToken, issueKey string) (*string, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", jiraBaseURL, issueKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating JIRA request: %v", err)
	}

	req.SetBasicAuth(jiraEmail, jiraAPIToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error fetching JIRA issue: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed to fetch JIRA issue. Status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading JIRA response body: %v", err)
	}

	var issue JIRAIssue
	err = json.Unmarshal(body, &issue)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling JIRA response: %v", err)
	}

	return &issue.Fields.Summary, nil
}

func main() {
	flag.Parse()

	jiraBaseURL := os.Getenv("JIRA_BASE_URL")
	jiraEmail := os.Getenv("JIRA_EMAIL")
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")
	githubToken := os.Getenv("GITHUB_TOKEN")

	if jiraBaseURL == "" || jiraEmail == "" || jiraAPIToken == "" || githubToken == "" {
		log.Fatalf("Error: Required environment variables (JIRA_BASE_URL, JIRA_EMAIL, JIRA_API_TOKEN, GITHUB_TOKEN) are not set.")
	}

	if *jiraIssueKey == "" {
		log.Fatalf("Error: The JIRA issue key must be provided using the -issue flag.")
	}

	sourceBranch, err := getCurrentBranch()
	if baseBranch == nil || *baseBranch == "" {
		baseBranch, err = getDefaultBranch()
	}
	if err != nil {
		log.Fatalf("Error getting current branch: %v", err)
	}
	fmt.Printf("Using current branch as source branch: %s\n", *sourceBranch)

	githubOwner, githubRepo, err := getGitHubRepoDetails()
	if err != nil {
		log.Fatalf("Error fetching GitHub repo details: %v", err)
	}
	fmt.Printf("GitHub Owner: %s, Repo: %s\n", *githubOwner, *githubRepo)

	prTitle, err := getJiraIssueTitle(jiraBaseURL, jiraEmail, jiraAPIToken, *jiraIssueKey)
	if err != nil {
		log.Fatalf("Error fetching JIRA issue: %v", err)
	}
	fmt.Printf("Fetched JIRA issue title: %s\n", *prTitle)

	githubURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", *githubOwner, *githubRepo)

	pr := PullRequest{
		Title: fmt.Sprintf("[%s] %s", *jiraIssueKey, *prTitle),
		Body:  fmt.Sprintf("%s/browse/%s", jiraBaseURL, *jiraIssueKey),
		Head:  *sourceBranch,
		Base:  *baseBranch,
		Draft: true,
	}

	prJSON, err := json.Marshal(pr)
	if err != nil {
		log.Fatalf("Error marshaling PR JSON: %v", err)
	}

	req, err := http.NewRequest("POST", githubURL, bytes.NewBuffer(prJSON))
	if err != nil {
		log.Fatalf("Error creating GitHub request: %v", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", githubToken))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending GitHub PR request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated {
		var prResp PullRequestResponse
		err = json.Unmarshal(body, &prResp)
		fmt.Println("Pull request created successfully!")
		fmt.Println(prResp.URL)
	} else {
		log.Fatalf("Failed to create pull request. Status: %s, Response: %s", resp.Status, string(body))
	}
}
