package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// GitHubProvider implements OAuthProvider using real GitHub OAuth2.
type GitHubProvider struct {
	config *oauth2.Config
}

// NewGitHubProvider creates a provider for GitHub OAuth2.
func NewGitHubProvider(clientID, clientSecret, redirectURL string) *GitHubProvider {
	return &GitHubProvider{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user"},
			Endpoint:     endpoints.GitHub,
		},
	}
}

func (g *GitHubProvider) AuthCodeURL(state string) string {
	return g.config.AuthCodeURL(state)
}

func (g *GitHubProvider) Exchange(ctx context.Context, code string) (GitHubUser, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return GitHubUser{}, fmt.Errorf("exchanging code: %w", err)
	}

	client := g.config.Client(ctx, token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return GitHubUser{}, fmt.Errorf("fetching github user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GitHubUser{}, fmt.Errorf("github user API returned %d", resp.StatusCode)
	}

	var data struct {
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return GitHubUser{}, fmt.Errorf("decoding github user: %w", err)
	}

	return GitHubUser{
		Username:   data.Login,
		Name:       data.Name,
		AvatarURL:  data.AvatarURL,
		ProfileURL: data.HTMLURL,
	}, nil
}
