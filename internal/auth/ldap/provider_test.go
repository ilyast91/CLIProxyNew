package ldap

import (
	"context"
	"errors"
	"testing"

	ldaplib "github.com/go-ldap/ldap/v3"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
)

func TestProviderAuthenticatesAdminAndEscapesUsername(t *testing.T) {
	connection := &fakeConnection{searchResult: &ldaplib.SearchResult{Entries: []*ldaplib.Entry{
		ldaplib.NewEntry("CN=Alice,OU=Users,DC=example,DC=test", map[string][]string{
			"mail":     {"alice@example.test"},
			"memberOf": {"CN=cliproxy-users,OU=Groups,DC=example,DC=test", "CN=cliproxy-admins,OU=Groups,DC=example,DC=test"},
		}),
	}}}
	provider, err := NewProvider(testConfig(), fakeDialer{connection: connection})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	got, err := provider.Authenticate(context.Background(), "alice*)(uid=*)", "secret")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.Username != "alice*)(uid=*)" || got.Email != "alice@example.test" || got.Role != identity.RoleAdmin || got.Source != identity.SourceLDAP {
		t.Fatalf("Authenticate() identity = %+v", got)
	}
	if len(connection.binds) != 2 || connection.binds[0] != "CN=svc,DC=example,DC=test:bind-secret" || connection.binds[1] != "CN=Alice,OU=Users,DC=example,DC=test:secret" {
		t.Fatalf("Bind calls = %v", connection.binds)
	}
	if connection.searchRequest.Filter != "(uid=alice\\2a\\29\\28uid=\\2a\\29)" {
		t.Fatalf("search filter = %q", connection.searchRequest.Filter)
	}
}

func TestProviderRejectsReservedNamespaceAndMissingGroups(t *testing.T) {
	t.Run("reserved namespace", func(t *testing.T) {
		provider, err := NewProvider(testConfig(), fakeDialer{})
		if err != nil {
			t.Fatalf("NewProvider() error = %v", err)
		}
		_, err = provider.Authenticate(context.Background(), "static:debug", "secret")
		if !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("Authenticate() error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("not in allowed groups", func(t *testing.T) {
		connection := &fakeConnection{searchResult: &ldaplib.SearchResult{Entries: []*ldaplib.Entry{
			ldaplib.NewEntry("CN=Bob,OU=Users,DC=example,DC=test", map[string][]string{"memberOf": {"CN=other,DC=example,DC=test"}}),
		}}}
		provider, err := NewProvider(testConfig(), fakeDialer{connection: connection})
		if err != nil {
			t.Fatalf("NewProvider() error = %v", err)
		}
		_, err = provider.Authenticate(context.Background(), "bob", "secret")
		if !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("Authenticate() error = %v, want ErrAccessDenied", err)
		}
	})
}

func testConfig() Config {
	return Config{
		URL:          "ldaps://ldap.example.test:636",
		BindDN:       "CN=svc,DC=example,DC=test",
		BindPassword: "bind-secret",
		UserBase:     "OU=Users,DC=example,DC=test",
		UserFilter:   "(uid={username})",
		UserGroupDN:  "CN=cliproxy-users,OU=Groups,DC=example,DC=test",
		AdminGroupDN: "CN=cliproxy-admins,OU=Groups,DC=example,DC=test",
	}
}

type fakeDialer struct {
	connection connection
	err        error
}

func (d fakeDialer) Dial(context.Context, string) (connection, error) {
	return d.connection, d.err
}

type fakeConnection struct {
	binds         []string
	searchRequest *ldaplib.SearchRequest
	searchResult  *ldaplib.SearchResult
	bindErr       error
	searchErr     error
}

func (c *fakeConnection) Bind(username, password string) error {
	c.binds = append(c.binds, username+":"+password)
	return c.bindErr
}

func (c *fakeConnection) Search(request *ldaplib.SearchRequest) (*ldaplib.SearchResult, error) {
	c.searchRequest = request
	return c.searchResult, c.searchErr
}

func (c *fakeConnection) Close() error { return nil }
