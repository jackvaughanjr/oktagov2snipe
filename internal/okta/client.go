package okta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Client is a lightweight Okta REST API client using SSWS token auth.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func NewClient(baseURL, apiToken string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UserProfile holds the profile fields returned by Okta.
type UserProfile struct {
	Login     string `json:"login"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// User represents an Okta user object.
type User struct {
	ID      string      `json:"id"`
	Status  string      `json:"status"`
	Profile UserProfile `json:"profile"`
}

// Role represents an Okta role assignment.
type Role struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

// ListActiveUsers returns all users with status ACTIVE, following pagination.
func (c *Client) ListActiveUsers(ctx context.Context) ([]User, error) {
	url := c.baseURL + "/api/v1/users?filter=status%20eq%20%22ACTIVE%22&limit=200"
	return c.listUsers(ctx, url)
}

// ListAllUsers returns all users regardless of status, following pagination.
func (c *Client) ListAllUsers(ctx context.Context) ([]User, error) {
	url := c.baseURL + "/api/v1/users?limit=200"
	return c.listUsers(ctx, url)
}

// GetUserByEmail fetches a single user by email/login.
func (c *Client) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	url := fmt.Sprintf("%s/api/v1/users/%s", c.baseURL, email)
	req, err := c.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("okta GetUserByEmail %s: status %d", email, resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserRoles returns roles for a user. HTTP 403 is treated as no roles (not an error).
func (c *Client) GetUserRoles(ctx context.Context, userID string) ([]Role, error) {
	url := fmt.Sprintf("%s/api/v1/users/%s/roles", c.baseURL, userID)
	req, err := c.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 403 means the caller doesn't have permission to list roles for this user;
	// treat as empty role list per spec.
	if resp.StatusCode == http.StatusForbidden {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("okta GetUserRoles %s: status %d", userID, resp.StatusCode)
	}

	var roles []Role
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// listUsers fetches all pages starting from startURL.
func (c *Client) listUsers(ctx context.Context, startURL string) ([]User, error) {
	var all []User
	nextURL := startURL

	for nextURL != "" {
		req, err := c.newRequest(ctx, http.MethodGet, nextURL)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("okta list users: status %d", resp.StatusCode)
		}

		var page []User
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		all = append(all, page...)
		nextURL = parseLinkNext(resp.Header.Get("Link"))
	}

	return all, nil
}

func (c *Client) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "SSWS "+c.apiToken)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// parseLinkNext extracts the URL from a Link header with rel="next".
// Returns "" if not present.
var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func parseLinkNext(header string) string {
	if m := linkNextRe.FindStringSubmatch(header); len(m) == 2 {
		return m[1]
	}
	return ""
}
