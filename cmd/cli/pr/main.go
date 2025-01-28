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
}

var (
	jiraIssueKey = flag.String("issue", "", "The JIRA issue key (e.g., PROJECT-123)")
	baseBranch   = flag.String("base", "main", "The base branch for the pull request")
)

// Function to fetch GitHub owner and repo from the remote origin URL
func getGitHubRepoDetails() (string, string, error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch remote origin URL: %w", err)
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
		return "", "", fmt.Errorf("could not parse owner and repo from URL: %s", url)
	}

	return owner, repo, nil
}

// Function to get the current branch name
func getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func getJiraIssueTitle(jiraBaseURL, jiraEmail, jiraAPIToken, issueKey string) (*string, error) {
	// Step 3: Fetch JIRA issue title
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
	// Define a flag for the JIRA issue key
	flag.Parse()

	// Read environment variables
	jiraBaseURL := os.Getenv("JIRA_BASE_URL")
	jiraEmail := os.Getenv("JIRA_EMAIL")
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")
	githubToken := os.Getenv("GITHUB_TOKEN")

	// Validate environment variables
	if jiraBaseURL == "" || jiraEmail == "" || jiraAPIToken == "" || githubToken == "" {
		log.Fatalf("Error: Required environment variables (JIRA_BASE_URL, JIRA_EMAIL, JIRA_API_TOKEN, GITHUB_TOKEN) are not set.")
	}

	// Validate the JIRA issue key
	if *jiraIssueKey == "" {
		log.Fatalf("Error: The JIRA issue key must be provided using the -issue flag.")
	}

	// Step 1: Get the current branch as the default source branch
	sourceBranch, err := getCurrentBranch()
	if err != nil {
		log.Fatalf("Error getting current branch: %v", err)
	}
	fmt.Printf("Using current branch as source branch: %s\n", sourceBranch)

	// Step 2: Fetch GitHub owner and repo details dynamically
	githubOwner, githubRepo, err := getGitHubRepoDetails()
	if err != nil {
		log.Fatalf("Error fetching GitHub repo details: %v", err)
	}
	fmt.Printf("GitHub Owner: %s, Repo: %s\n", githubOwner, githubRepo)

	prTitle, err := getJiraIssueTitle(jiraBaseURL, jiraEmail, jiraAPIToken, *jiraIssueKey)
	if err != nil {
		log.Fatalf("Error fetching JIRA issue: %v", err)
	}
	fmt.Printf("Fetched JIRA issue title: %s\n", *prTitle)

	// Step 4: Create Pull Request on GitHub
	githubURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", githubOwner, githubRepo)

	pr := PullRequest{
		Title: *prTitle,
		Body:  fmt.Sprintf("%s/browse/%s.", jiraBaseURL, *jiraIssueKey),
		Head:  sourceBranch,
		Base:  *baseBranch,
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
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending GitHub PR request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated {
		fmt.Println("Pull request created successfully!")
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Failed to create pull request. Status: %s, Response: %s", resp.Status, string(body))
	}
}
