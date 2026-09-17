// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	osuser "os/user"
	"strings"
	"unicode"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/initialize"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/auth"
	"code.vikunja.io/api/pkg/user"
	"github.com/spf13/cobra"
)

func init() {
	var output string
	var userID int64
	command := &cobra.Command{
		Use:   "tasktrace-local-session",
		Short: "Prepare a private session for the loopback-only TaskTrace experience",
		RunE: func(cmd *cobra.Command, _ []string) error {
			initialize.LightInit()
			host, _, err := net.SplitHostPort(config.ServiceInterface.GetString())
			if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || config.DatabaseType.GetString() != "sqlite" {
				return fmt.Errorf("local sessions require a loopback IP listener and SQLite")
			}
			initialize.FullInitWithoutAsync()
			s := db.NewSession()
			defer s.Close()
			defer events.CleanupPending(s)
			var selected *user.User
			if userID > 0 {
				selected, err = user.GetUserByID(s, userID)
				if err != nil {
					return fmt.Errorf("load local user: %w", err)
				}
			} else {
				var users []*user.User
				if err = s.Asc("id").Find(&users); err != nil {
					return fmt.Errorf("list local users: %w", err)
				}
				for _, u := range users {
					if u.Status != user.StatusActive || u.IsBot() || !u.DeletionScheduledAt.IsZero() {
						continue
					}
					if selected != nil {
						return fmt.Errorf("multiple users exist; set data/local-user-id.txt to the desired numeric user ID")
					}
					selected = u
				}
				if selected == nil && len(users) > 0 {
					return fmt.Errorf("no active local user is available")
				}
			}
			accountUsername, accountName := localSystemAccount()
			if selected == nil {
				password := make([]byte, 32)
				if _, err = rand.Read(password); err != nil {
					return fmt.Errorf("generate local password: %w", err)
				}
				selected, err = user.CreateUser(s, &user.User{
					Username: accountUsername,
					Name:     accountName,
					Email:    "local@tasktrace.invalid",
					Password: hex.EncodeToString(password),
					Issuer:   user.IssuerLocal,
					Language: "zh-CN",
				})
				if err != nil {
					return fmt.Errorf("create local workspace: %w", err)
				}
				if err = models.CreateNewProjectForUser(s, selected); err != nil {
					return fmt.Errorf("create local project: %w", err)
				}
			}
			if selected.IsBot() || !selected.DeletionScheduledAt.IsZero() || selected.Status != user.StatusActive {
				return fmt.Errorf("the selected account cannot be used for a local session")
			}
			// Keep the same owner ID, even when this portable workspace changes computers.
			// Only the local account's name fields change; task ownership stays intact.
			if accountName != "" {
				nextUsername := selected.Username
				if accountUsername != selected.Username {
					occupied, lookupErr := s.Where("username = ? AND id != ?", accountUsername, selected.ID).Exist(&user.User{})
					if lookupErr != nil {
						return fmt.Errorf("check local account name: %w", lookupErr)
					}
					if !occupied {
						nextUsername = accountUsername
					}
				}
				if selected.Name != accountName || selected.Username != nextUsername {
					selected.Name = accountName
					selected.Username = nextUsername
					if _, err = s.ID(selected.ID).Cols("name", "username").Update(selected); err != nil {
						return fmt.Errorf("sync local account name: %w", err)
					}
				}
			}
			if err = s.Commit(); err != nil {
				return fmt.Errorf("commit local workspace: %w", err)
			}
			session, err := auth.IssueUserToken(cmd.Context(), selected, "TaskTrace Local", "127.0.0.1", true, nil)
			if err != nil {
				return fmt.Errorf("issue local session: %w", err)
			}
			payload, err := json.Marshal(struct {
				UserID  int64  `json:"user_id"`
				Token   string `json:"token"`
				Refresh string `json:"refresh_token"`
			}{
				UserID:  selected.ID,
				Token:   session.AccessToken,
				Refresh: session.RefreshToken,
			})
			if err != nil {
				return fmt.Errorf("encode local session: %w", err)
			}
			if err = os.WriteFile(output, payload, 0600); err != nil {
				return fmt.Errorf("save local session: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&output, "output", "", "Private session output file")
	_ = command.MarkFlagRequired("output")
	command.Flags().Int64Var(&userID, "user-id", 0, "Existing workspace owner")
	rootCmd.AddCommand(command)
}

// Read the OS identity, not a name persisted in the portable package.
func localSystemAccount() (username, name string) {
	login := strings.TrimSpace(os.Getenv("USERNAME"))
	if current, err := osuser.Current(); err == nil {
		login = current.Username
		name = strings.TrimSpace(current.Name)
	}
	if index := strings.LastIndexAny(login, `\/`); index >= 0 {
		login = login[index+1:]
	}
	login = strings.TrimSpace(login)
	if name == "" {
		name = login
	}
	if login == "" {
		login = "tasktrace-local"
	}
	username = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, login)
	if strings.HasPrefix(username, "bot-") || strings.HasPrefix(username, "link-share-") {
		username = "local-" + username
	}
	if len([]rune(username)) > 250 {
		username = string([]rune(username)[:250])
	}
	return username, name
}
