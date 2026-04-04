package auth

import "context"

// FakeProvider is an OAuthProvider for testing that returns controlled responses.
type FakeProvider struct {
	User GitHubUser
	Err  error
	URL  string
}

func (f *FakeProvider) AuthCodeURL(state string) string {
	return f.URL + "?state=" + state
}

func (f *FakeProvider) Exchange(_ context.Context, _ string) (GitHubUser, error) {
	if f.Err != nil {
		return GitHubUser{}, f.Err
	}
	return f.User, nil
}
