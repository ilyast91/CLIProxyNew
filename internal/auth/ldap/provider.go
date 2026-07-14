// Package ldap реализует LDAP identity provider для production-режима.
package ldap

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	ldaplib "github.com/go-ldap/ldap/v3"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
)

const staticUsernamePrefix = "static:"

var (
	// ErrAccessDenied означает, что LDAP-пользователь не состоит в разрешённой группе.
	ErrAccessDenied = identity.ErrAccessDenied
	// ErrInvalidConfiguration означает небезопасную или неполную LDAP-конфигурацию.
	ErrInvalidConfiguration = errors.New("некорректная LDAP-конфигурация")
)

// Config описывает параметры LDAP source. BindPassword приходит только из env.
type Config struct {
	URL          string
	BindDN       string
	BindPassword string
	UserBase     string
	UserFilter   string
	UserGroupDN  string
	AdminGroupDN string
}

// Provider реализует identity.Provider через service-bind, search и user-bind.
type Provider struct {
	config Config
	dialer dialer
}

type dialer interface {
	Dial(ctx context.Context, address string) (connection, error)
}

type connection interface {
	Bind(username, password string) error
	Search(request *ldaplib.SearchRequest) (*ldaplib.SearchResult, error)
	Close() error
}

type networkDialer struct{}

func (networkDialer) Dial(ctx context.Context, address string) (connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := ldaplib.DialURL(address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// NewProvider создаёт LDAP provider с проверенной TLS-конфигурацией.
func NewProvider(config Config, customDialer dialer) (*Provider, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if customDialer == nil {
		customDialer = networkDialer{}
	}
	return &Provider{config: config, dialer: customDialer}, nil
}

// Authenticate выполняет service-bind, поиск, user-bind и проверку LDAP-групп.
func (p *Provider) Authenticate(ctx context.Context, username, password string) (identity.Identity, error) {
	if p == nil || p.dialer == nil {
		return identity.Identity{}, fmt.Errorf("LDAP provider is not configured")
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" || strings.HasPrefix(username, staticUsernamePrefix) {
		return identity.Identity{}, identity.ErrInvalidCredentials
	}

	conn, err := p.dialer.Dial(ctx, p.config.URL)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("dial LDAP: %w", err)
	}
	defer conn.Close()

	if err := conn.Bind(p.config.BindDN, p.config.BindPassword); err != nil {
		return identity.Identity{}, fmt.Errorf("LDAP service bind: %w", err)
	}
	entry, err := p.findUser(ctx, conn, username)
	if err != nil {
		return identity.Identity{}, err
	}
	if err := conn.Bind(entry.DN, password); err != nil {
		return identity.Identity{}, identity.ErrInvalidCredentials
	}

	role, err := p.roleForGroups(entry.GetAttributeValues("memberOf"))
	if err != nil {
		return identity.Identity{}, err
	}
	return identity.Identity{
		Username: username,
		Email:    entry.GetAttributeValue("mail"),
		Role:     role,
		Source:   identity.SourceLDAP,
	}, nil
}

func (p *Provider) findUser(ctx context.Context, conn connection, username string) (*ldaplib.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filter := strings.Replace(p.config.UserFilter, "{username}", ldaplib.EscapeFilter(username), 1)
	result, err := conn.Search(ldaplib.NewSearchRequest(
		p.config.UserBase,
		ldaplib.ScopeWholeSubtree,
		ldaplib.NeverDerefAliases,
		2,
		0,
		false,
		filter,
		[]string{"mail", "memberOf"},
		nil,
	))
	if err != nil {
		return nil, fmt.Errorf("search LDAP user: %w", err)
	}
	if result == nil || len(result.Entries) != 1 {
		return nil, identity.ErrInvalidCredentials
	}
	return result.Entries[0], nil
}

func (p *Provider) roleForGroups(groups []string) (string, error) {
	if includesDN(groups, p.config.AdminGroupDN) {
		return identity.RoleAdmin, nil
	}
	if includesDN(groups, p.config.UserGroupDN) {
		return identity.RoleUser, nil
	}
	return "", ErrAccessDenied
}

func includesDN(groups []string, wanted string) bool {
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group), strings.TrimSpace(wanted)) {
			return true
		}
	}
	return false
}

func validateConfig(config Config) error {
	endpoint, err := url.Parse(config.URL)
	if err != nil || endpoint.Scheme != "ldaps" || endpoint.Host == "" {
		return fmt.Errorf("%w: ldap.url must use ldaps://", ErrInvalidConfiguration)
	}
	if strings.TrimSpace(config.BindDN) == "" || config.BindPassword == "" ||
		strings.TrimSpace(config.UserBase) == "" || strings.Count(config.UserFilter, "{username}") != 1 ||
		strings.TrimSpace(config.UserGroupDN) == "" || strings.TrimSpace(config.AdminGroupDN) == "" {
		return ErrInvalidConfiguration
	}
	return nil
}
