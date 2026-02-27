package scm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractPackageInfo(t *testing.T) {
	tests := []struct {
		name            string
		title           string
		expectedPackage string
		expectedOrg     string
	}{
		// Standard Go packages
		{
			name:            "GitHub Go package with org",
			title:           "Bump github.com/datadog/datadog-go from 1.0.0 to 2.0.0",
			expectedPackage: "github.com/datadog/datadog-go",
			expectedOrg:     "datadog",
		},
		{
			name:            "GitHub package without version",
			title:           "Update github.com/elastic/go-elasticsearch to v8",
			expectedPackage: "github.com/elastic/go-elasticsearch",
			expectedOrg:     "elastic",
		},
		{
			name:            "Gopkg.in package",
			title:           "Bump gopkg.in/mgo.v2 from 2.0.0 to 2.0.1",
			expectedPackage: "gopkg.in/mgo.v2",
			expectedOrg:     "",
		},
		{
			name:            "Golang.org/x package",
			title:           "Update golang.org/x/net from v0.1.0 to v0.2.0",
			expectedPackage: "golang.org/x/net",
			expectedOrg:     "", // golang.org/x packages don't have an org
		},
		{
			name:            "Package with hashicorp org",
			title:           "Bump github.com/hashicorp/consul from 1.10.0 to 1.11.0",
			expectedPackage: "github.com/hashicorp/consul",
			expectedOrg:     "hashicorp",
		},
		{
			name:            "AWS SDK v1",
			title:           "Update github.com/aws/aws-sdk-go from v1.44.0 to v1.45.0",
			expectedPackage: "github.com/aws/aws-sdk-go",
			expectedOrg:     "aws",
		},
		{
			name:            "AWS SDK v2",
			title:           "Bump github.com/aws/aws-sdk-go-v2 from v1.16.0 to v1.17.0",
			expectedPackage: "github.com/aws/aws-sdk-go-v2",
			expectedOrg:     "aws",
		},
		{
			name:            "Deprecated pkg/errors",
			title:           "Update github.com/pkg/errors from 0.9.0 to 0.9.1",
			expectedPackage: "github.com/pkg/errors",
			expectedOrg:     "pkg",
		},
		{
			name:            "JWT package old",
			title:           "Bump github.com/dgrijalva/jwt-go from v3.2.0 to v3.2.1",
			expectedPackage: "github.com/dgrijalva/jwt-go",
			expectedOrg:     "dgrijalva",
		},
		{
			name:            "JWT package new",
			title:           "Update github.com/golang-jwt/jwt from v4.4.0 to v4.5.0",
			expectedPackage: "github.com/golang-jwt/jwt",
			expectedOrg:     "golang-jwt",
		},
		{
			name:            "Gorilla mux",
			title:           "Bump github.com/gorilla/mux from v1.8.0 to v1.8.1",
			expectedPackage: "github.com/gorilla/mux",
			expectedOrg:     "gorilla",
		},
		{
			name:            "Logrus package",
			title:           "Update github.com/sirupsen/logrus from v1.8.0 to v1.9.0",
			expectedPackage: "github.com/sirupsen/logrus",
			expectedOrg:     "sirupsen",
		},
		{
			name:            "Go-kit package",
			title:           "Bump github.com/go-kit/kit from v0.12.0 to v0.13.0",
			expectedPackage: "github.com/go-kit/kit",
			expectedOrg:     "go-kit",
		},
		{
			name:            "Gin v1",
			title:           "Update github.com/gin-gonic/gin from v1.7.0 to v1.8.0",
			expectedPackage: "github.com/gin-gonic/gin",
			expectedOrg:     "gin-gonic",
		},
		{
			name:            "NewRelic APM",
			title:           "Bump github.com/newrelic/go-agent from v3.15.0 to v3.16.0",
			expectedPackage: "github.com/newrelic/go-agent",
			expectedOrg:     "newrelic",
		},
		{
			name:            "Chore deps format",
			title:           "chore(deps): bump github.com/spf13/cobra from 1.6.0 to 1.7.0",
			expectedPackage: "github.com/spf13/cobra",
			expectedOrg:     "spf13",
		},
		// Real-world Dependabot formats with emoji
		{
			name:            "Emoji format - golang.org/x/tools",
			title:           "⬆️ (deps): Bump golang.org/x/tools from 0.36.0 to 0.37.0",
			expectedPackage: "golang.org/x/tools",
			expectedOrg:     "",
		},
		{
			name:            "Emoji format - redis client",
			title:           "⬆️ (deps): Bump github.com/redis/go-redis/v9 from 9.13.0 to 9.14.0",
			expectedPackage: "github.com/redis/go-redis/v9",
			expectedOrg:     "redis",
		},
		{
			name:            "Emoji format - AWS SDK group",
			title:           "⬆️ (deps): Bump the aws-sdk-go-v2 group with 4 updates",
			expectedPackage: "aws-sdk-go-v2",
			expectedOrg:     "",
		},
		{
			name:            "Emoji format - DataDog tracer",
			title:           "⬆️ (deps): bump gopkg.in/DataDog/dd-trace-go.v1 from 1.73.1 to 1.74.2",
			expectedPackage: "gopkg.in/DataDog/dd-trace-go.v1",
			expectedOrg:     "datadog", // gopkg.in/DataDog extracts to datadog
		},
		{
			name:            "Emoji format - AWS SDK v1",
			title:           "⬆️ (deps): bump github.com/aws/aws-sdk-go from 1.55.7 to 1.55.8",
			expectedPackage: "github.com/aws/aws-sdk-go",
			expectedOrg:     "aws",
		},
		{
			name:            "Emoji format - protobuf",
			title:           "⬆️ (deps): Bump google.golang.org/protobuf from 1.36.8 to 1.36.9",
			expectedPackage: "google.golang.org/protobuf",
			expectedOrg:     "",
		},
		{
			name:            "Emoji format - chi router",
			title:           "⬆️ (deps): Bump github.com/go-chi/chi/v5 from 5.2.2 to 5.2.3",
			expectedPackage: "github.com/go-chi/chi/v5",
			expectedOrg:     "go-chi",
		},
		{
			name:            "Emoji format - mockery",
			title:           "⬆️ (deps): Bump github.com/vektra/mockery/v2 from 2.53.4 to 2.53.5",
			expectedPackage: "github.com/vektra/mockery/v2",
			expectedOrg:     "vektra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, org := extractPackageInfo(tt.title)
			if pkg != tt.expectedPackage {
				t.Errorf("extractPackageInfo() package = %v, want %v", pkg, tt.expectedPackage)
			}
			if org != tt.expectedOrg {
				t.Errorf("extractPackageInfo() org = %v, want %v", org, tt.expectedOrg)
			}
		})
	}
}

func TestIsDenied(t *testing.T) {
	// Denied packages from config.example.yaml
	deniedPackages := []string{
		"github.com/pkg/errors",
		"github.com/dgrijalva/jwt-go",
		"github.com/gorilla/mux",
		"gopkg.in/mgo.v2",
		"github.com/sirupsen/logrus",
		"github.com/go-kit/kit",
		"github.com/gin-gonic/gin@v1",
		"github.com/aws/aws-sdk-go",
		"golang.org/x/net",
		"*alpha*",
		"*beta*",
		"*rc*",
		"*/v0",
	}

	// Denied orgs from config.example.yaml
	deniedOrgs := []string{
		"datadog",
		"elastic",
		"newrelic",
		"hashicorp",
	}

	tests := []struct {
		name        string
		packageName string
		orgName     string
		shouldDeny  bool
		reason      string
	}{
		// Direct package matches
		{
			name:        "Exact match - pkg/errors",
			packageName: "github.com/pkg/errors",
			orgName:     "pkg",
			shouldDeny:  true,
			reason:      "Exact package match",
		},
		{
			name:        "Exact match - jwt-go",
			packageName: "github.com/dgrijalva/jwt-go",
			orgName:     "dgrijalva",
			shouldDeny:  true,
			reason:      "Exact package match",
		},
		{
			name:        "Exact match - gorilla/mux",
			packageName: "github.com/gorilla/mux",
			orgName:     "gorilla",
			shouldDeny:  true,
			reason:      "Exact package match",
		},
		{
			name:        "Exact match - mgo.v2",
			packageName: "gopkg.in/mgo.v2",
			orgName:     "",
			shouldDeny:  true,
			reason:      "Exact package match",
		},

		// Organization matches
		{
			name:        "Org match - datadog",
			packageName: "github.com/datadog/datadog-go",
			orgName:     "datadog",
			shouldDeny:  true,
			reason:      "Organization is denied",
		},
		{
			name:        "Org match - elastic",
			packageName: "github.com/elastic/go-elasticsearch",
			orgName:     "elastic",
			shouldDeny:  true,
			reason:      "Organization is denied",
		},
		{
			name:        "Org match - newrelic",
			packageName: "github.com/newrelic/go-agent",
			orgName:     "newrelic",
			shouldDeny:  true,
			reason:      "Organization is denied",
		},
		{
			name:        "Org match - hashicorp",
			packageName: "github.com/hashicorp/terraform",
			orgName:     "hashicorp",
			shouldDeny:  true,
			reason:      "Organization is denied",
		},

		// Pattern matches
		{
			name:        "Pattern match - alpha version",
			packageName: "github.com/some/package-alpha",
			orgName:     "some",
			shouldDeny:  true,
			reason:      "Matches *alpha* pattern",
		},
		{
			name:        "Pattern match - beta version",
			packageName: "github.com/some/lib-beta",
			orgName:     "some",
			shouldDeny:  true,
			reason:      "Matches *beta* pattern",
		},
		{
			name:        "Pattern match - rc version",
			packageName: "github.com/some/tool-rc1",
			orgName:     "some",
			shouldDeny:  true,
			reason:      "Matches *rc* pattern",
		},
		{
			name:        "Pattern match - v0 package",
			packageName: "github.com/experimental/api/v0",
			orgName:     "experimental",
			shouldDeny:  true,
			reason:      "Matches */v0 pattern",
		},

		// Partial matches
		{
			name:        "Partial match - gin v1",
			packageName: "github.com/gin-gonic/gin@v1.7.0",
			orgName:     "gin-gonic",
			shouldDeny:  true,
			reason:      "Contains gin-gonic/gin@v1",
		},
		{
			name:        "AWS SDK v1 denied",
			packageName: "github.com/aws/aws-sdk-go",
			orgName:     "aws",
			shouldDeny:  true,
			reason:      "Exact match for aws-sdk-go v1",
		},

		// Should NOT be denied
		{
			name:        "Allowed - jwt v4",
			packageName: "github.com/golang-jwt/jwt",
			orgName:     "golang-jwt",
			shouldDeny:  false,
			reason:      "New JWT package is allowed",
		},
		{
			name:        "Allowed - gin v2",
			packageName: "github.com/gin-gonic/gin@v2.0.0",
			orgName:     "gin-gonic",
			shouldDeny:  false,
			reason:      "Gin v2 is allowed",
		},
		{
			name:        "Allowed - aws-sdk-go-v2",
			packageName: "github.com/aws/aws-sdk-go-v2",
			orgName:     "aws",
			shouldDeny:  false,
			reason:      "AWS SDK v2 is allowed",
		},
		{
			name:        "Allowed - stable version",
			packageName: "github.com/spf13/cobra",
			orgName:     "spf13",
			shouldDeny:  false,
			reason:      "Stable package not in deny list",
		},
		{
			name:        "Allowed - v1 package",
			packageName: "github.com/some/api/v1",
			orgName:     "some",
			shouldDeny:  false,
			reason:      "v1 packages are allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDenied(tt.packageName, tt.orgName, deniedPackages, deniedOrgs)
			if result != tt.shouldDeny {
				t.Errorf("isDenied() = %v, want %v (reason: %s)", result, tt.shouldDeny, tt.reason)
			}
		})
	}
}

func TestRealWorldDenials(t *testing.T) {
	// Test with real-world PR titles and deny lists
	deniedPackages := []string{
		"github.com/aws/aws-sdk-go", // Deny v1, but not v2
	}
	deniedOrgs := []string{
		"datadog",
	}

	tests := []struct {
		prTitle    string
		shouldDeny bool
		reason     string
	}{
		{
			prTitle:    "⬆️ (deps): bump gopkg.in/DataDog/dd-trace-go.v1 from 1.73.1 to 1.74.2",
			shouldDeny: true,
			reason:     "DataDog packages should be denied",
		},
		{
			prTitle:    "⬆️ (deps): bump github.com/aws/aws-sdk-go from 1.55.7 to 1.55.8",
			shouldDeny: true,
			reason:     "AWS SDK v1 should be denied",
		},
		{
			prTitle:    "⬆️ (deps): Bump the aws-sdk-go-v2 group with 4 updates",
			shouldDeny: false,
			reason:     "AWS SDK v2 should be allowed",
		},
		{
			prTitle:    "⬆️ (deps): Bump github.com/redis/go-redis/v9 from 9.13.0 to 9.14.0",
			shouldDeny: false,
			reason:     "Redis client should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.prTitle, func(t *testing.T) {
			pkg, org := extractPackageInfo(tt.prTitle)
			result := isDenied(pkg, org, deniedPackages, deniedOrgs)
			if result != tt.shouldDeny {
				t.Errorf("For PR '%s': isDenied() = %v, want %v (reason: %s, extracted pkg: %s, org: %s)",
					tt.prTitle, result, tt.shouldDeny, tt.reason, pkg, org)
			}
		})
	}
}

func TestIsDeniedCaseInsensitive(t *testing.T) {
	deniedPackages := []string{
		"github.com/pkg/errors",
	}
	deniedOrgs := []string{
		"DataDog",
	}

	tests := []struct {
		name        string
		packageName string
		orgName     string
		shouldDeny  bool
	}{
		{
			name:        "Package - different case",
			packageName: "GitHub.com/PKG/Errors",
			orgName:     "PKG",
			shouldDeny:  true,
		},
		{
			name:        "Org - different case",
			packageName: "github.com/datadog/dd-trace-go",
			orgName:     "datadog",
			shouldDeny:  true,
		},
		{
			name:        "Org - uppercase",
			packageName: "github.com/DATADOG/dd-trace-go",
			orgName:     "DATADOG",
			shouldDeny:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDenied(tt.packageName, tt.orgName, deniedPackages, deniedOrgs)
			if result != tt.shouldDeny {
				t.Errorf("isDenied() = %v, want %v", result, tt.shouldDeny)
			}
		})
	}
}

func TestWildcardPatterns(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		packageName string
		shouldMatch bool
	}{
		{
			name:        "Alpha in package name",
			pattern:     "*alpha*",
			packageName: "github.com/example/tool-alpha",
			shouldMatch: true,
		},
		{
			name:        "Alpha in middle",
			pattern:     "*alpha*",
			packageName: "github.com/example/alpha-tool",
			shouldMatch: true,
		},
		{
			name:        "Beta suffix",
			pattern:     "*beta*",
			packageName: "github.com/example/lib-v2-beta",
			shouldMatch: true,
		},
		{
			name:        "RC with number",
			pattern:     "*rc*",
			packageName: "github.com/example/app-rc1",
			shouldMatch: true,
		},
		{
			name:        "v0 at end",
			pattern:     "*/v0",
			packageName: "github.com/experimental/api/v0",
			shouldMatch: true,
		},
		{
			name:        "v0 not at end",
			pattern:     "*/v0",
			packageName: "github.com/experimental/v0/api",
			shouldMatch: false,
		},
		{
			name:        "Not v0",
			pattern:     "*/v0",
			packageName: "github.com/stable/api/v1",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deniedPackages := []string{tt.pattern}
			result := isDenied(tt.packageName, "", deniedPackages, []string{})
			if result != tt.shouldMatch {
				t.Errorf("Pattern %s match for %s = %v, want %v",
					tt.pattern, tt.packageName, result, tt.shouldMatch)
			}
		})
	}
}

func TestEnableAutoMerge(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		responseStatus int
		expectError    bool
	}{
		{
			name: "successful auto-merge enable",
			responseBody: `{
				"data": {
					"enablePullRequestAutoMerge": {
						"pullRequest": {
							"autoMergeRequest": {
								"enabledAt": "2026-01-01T00:00:00Z"
							}
						}
					}
				}
			}`,
			responseStatus: 200,
			expectError:    false,
		},
		{
			name: "auto-merge not allowed on repo",
			responseBody: `{
				"data": null,
				"errors": [
					{
						"message": "Pull request is not in the correct state to enable auto-merge"
					}
				]
			}`,
			responseStatus: 200,
			expectError:    true,
		},
		{
			name:           "server error",
			responseBody:   `{"message": "Internal Server Error"}`,
			responseStatus: 500,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
				}

				body, _ := io.ReadAll(r.Body)
				json.Unmarshal(body, &receivedBody)

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			c := NewGithubClient(http.DefaultClient, "test-token")
			ctx := context.Background()

			err := c.EnableAutoMerge(ctx, server.URL, DependencyUpdateRequest{
				Owner:             "owner",
				Repo:              "repo",
				PullRequestNumber: 42,
				NodeID:            "PR_abc123",
				Title:             "Bump foo from 1.0 to 2.0",
				PackageName:       "foo",
			})

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if receivedBody != nil {
				query, ok := receivedBody["query"].(string)
				if !ok || !strings.Contains(query, "enablePullRequestAutoMerge") {
					t.Error("request body missing enablePullRequestAutoMerge mutation")
				}
				variables, ok := receivedBody["variables"].(map[string]interface{})
				if !ok {
					t.Fatal("request body missing variables")
				}
				if variables["pullRequestId"] != "PR_abc123" {
					t.Errorf("expected pullRequestId PR_abc123, got %v", variables["pullRequestId"])
				}
				if variables["mergeMethod"] != "SQUASH" {
					t.Errorf("expected mergeMethod SQUASH, got %v", variables["mergeMethod"])
				}
			}
		})
	}
}
