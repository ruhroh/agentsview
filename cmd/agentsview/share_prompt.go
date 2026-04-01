package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wesm/agentsview/internal/config"
)

func ensureShareConfigOnStartup(cfg *config.Config) error {
	return ensureShareConfigOnStartupWithIO(
		cfg,
		os.Stdin,
		os.Stdout,
		isInteractiveTerminal(os.Stdin) && isInteractiveTerminal(os.Stdout),
	)
}

func ensureShareConfigOnStartupWithIO(
	cfg *config.Config,
	in io.Reader,
	out io.Writer,
	interactive bool,
) error {
	persisted, err := cfg.PersistedShareConfig()
	if err != nil {
		return err
	}

	missingPersisted := missingShareFields(persisted)
	if len(missingPersisted) == 0 {
		return nil
	}

	if len(missingShareFields(cfg.Share)) == 0 {
		return cfg.SaveShareConfig(cfg.Share)
	}

	if !interactive {
		fmt.Fprintf(
			out,
			"warning: share config is incomplete (%s); run agentsview in an interactive terminal to configure it\n",
			strings.Join(missingShareFields(cfg.Share), ", "),
		)
		return nil
	}

	return promptForMissingShareConfig(cfg, in, out)
}

func promptForMissingShareConfig(
	cfg *config.Config,
	in io.Reader,
	out io.Writer,
) error {
	reader := bufio.NewReader(in)

	fmt.Fprintln(out, "Share publishing is not fully configured.")
	configure, err := promptYesNo(
		reader,
		out,
		"Configure share.url, share.token, and share.publisher now?",
		true,
	)
	if err != nil {
		return err
	}
	if !configure {
		fmt.Fprintln(out, "Skipping share configuration.")
		return nil
	}

	share := cfg.Share
	if share.Publisher == "" {
		if host, err := os.Hostname(); err == nil {
			share.Publisher = host
		}
	}

	if share.URL == "" {
		share.URL, err = promptRequiredValue(
			reader,
			out,
			"Share server URL",
			share.URL,
			func(v string) string {
				return strings.TrimRight(strings.TrimSpace(v), "/")
			},
		)
		if err != nil {
			return err
		}
	}
	if share.Token == "" {
		share.Token, err = promptRequiredValue(
			reader,
			out,
			"Share auth token",
			share.Token,
			strings.TrimSpace,
		)
		if err != nil {
			return err
		}
	}
	if cfg.Share.Publisher == "" {
		share.Publisher, err = promptRequiredValue(
			reader,
			out,
			"Share publisher",
			share.Publisher,
			strings.TrimSpace,
		)
		if err != nil {
			return err
		}
	}

	if err := cfg.SaveShareConfig(share); err != nil {
		return err
	}
	fmt.Fprintf(out, "Saved share configuration to %s\n", filepath.Join(cfg.DataDir, "config.toml"))
	return nil
}

func promptYesNo(
	reader *bufio.Reader,
	out io.Writer,
	label string,
	defaultYes bool,
) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	for {
		fmt.Fprintf(out, "%s %s ", label, suffix)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				return false, nil
			}
			return false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer == "" {
			return defaultYes, nil
		}
		switch answer {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(out, `Enter "y" or "n".`)
	}
}

func promptRequiredValue(
	reader *bufio.Reader,
	out io.Writer,
	label string,
	current string,
	normalize func(string) string,
) (string, error) {
	for {
		if current != "" {
			fmt.Fprintf(out, "%s [%s]: ", label, current)
		} else {
			fmt.Fprintf(out, "%s: ", label)
		}

		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return "", err
		}

		value := normalize(line)
		if value == "" && current != "" {
			return current, nil
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintf(out, "%s is required.\n", label)
	}
}

func missingShareFields(share config.ShareConfig) []string {
	var missing []string
	if share.URL == "" {
		missing = append(missing, "share.url")
	}
	if share.Token == "" {
		missing = append(missing, "share.token")
	}
	if share.Publisher == "" {
		missing = append(missing, "share.publisher")
	}
	return missing
}

func isInteractiveTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
