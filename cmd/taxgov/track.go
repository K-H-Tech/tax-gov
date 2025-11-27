package main

import (
	"fmt"

	"github.com/K-H-Tech/auto-tax-gov/internal/config"
	"github.com/K-H-Tech/auto-tax-gov/internal/tracker"
	"github.com/spf13/cobra"
)

var (
	outputDir  string
	outputJSON bool
)

var trackCmd = &cobra.Command{
	Use:   "track",
	Short: "Track redirects from the command line",
	Long: `Track HTTP redirects on the tax portal and save results to files.

This command performs the redirect chain tracking without starting
a web server. Results are saved to markdown and JSON files.`,
	RunE: runTrack,
}

func init() {
	trackCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "directory to save results")
	trackCmd.Flags().BoolVarP(&outputJSON, "json", "j", false, "output results as JSON")

	rootCmd.AddCommand(trackCmd)
}

func runTrack(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	t := tracker.New(cfg, logger)

	logger.Info("starting redirect tracking", "startURL", cfg.GetStartURL())

	steps, _, err := t.TrackRedirects(cfg.GetStartURL(), nil)
	if err != nil {
		return fmt.Errorf("tracking failed: %w", err)
	}

	logger.Info("tracking complete", "steps", len(steps))

	// Save results
	if err := t.SaveResults(steps); err != nil {
		return fmt.Errorf("failed to save results: %w", err)
	}

	// Print summary
	fmt.Printf("\nRedirect Chain Summary:\n")
	fmt.Printf("=======================\n")
	for i, step := range steps {
		fmt.Printf("%d. [%d] %s\n", i+1, step.StatusCode, step.URL)
		if step.RedirectLocation != "" {
			fmt.Printf("   → %s\n", step.RedirectLocation)
		}
	}
	fmt.Printf("\nTotal steps: %d\n", len(steps))

	// Check for captcha
	for _, step := range steps {
		if step.CaptchaData != nil {
			fmt.Printf("\nCaptcha found at: %s\n", step.URL)
			fmt.Printf("CSRF Token: %s\n", step.CaptchaData.CSRFToken[:min(20, len(step.CaptchaData.CSRFToken))])
			break
		}
	}

	return nil
}
