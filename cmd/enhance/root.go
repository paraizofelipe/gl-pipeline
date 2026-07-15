package enhance

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	slog "log"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/charmbracelet/fang"
	"github.com/dlvhdr/gh-enhance/internal/tui"
	"github.com/dlvhdr/gh-enhance/internal/version"
)

//go:embed logo.txt
var asciiArt string
var logoWithTagline = lipgloss.NewStyle().Foreground(lipgloss.Green).Render(asciiArt)

var rootCmd = &cobra.Command{
	Use:   "gl-pipeline [<MR URL> | <MR IID> | <pipeline URL>] [flags]",
	Long:  logoWithTagline,
	Short: "A Blazingly Fast Terminal UI for GitLab CI Pipelines",
	Args:  cobra.MinimumNArgs(0),
	Example: `# view the latest pipeline of a merge request by URL
 gl-pipeline https://gitlab.com/group/project/-/merge_requests/42

 # view a merge request's pipeline by IID (inside a clone of the project)
 gl-pipeline 42

 # view a specific pipeline by URL
 gl-pipeline https://gitlab.com/group/project/-/pipelines/12345

 # view a specific pipeline by ID
 gl-pipeline --pipeline 12345 -R group/project

 # no args: the latest pipeline for the current branch
 gl-pipeline`,
}

func Execute() error {
	themeFunc := fang.WithColorSchemeFunc(func(
		ld lipgloss.LightDarkFunc,
	) fang.ColorScheme {
		def := fang.DefaultColorScheme(ld)
		def.DimmedArgument = ld(lipgloss.Black, lipgloss.White)
		def.Codeblock = ld(lipgloss.Color("#F1EFEF"), lipgloss.Color("#141417"))
		def.Title = lipgloss.Green
		def.Command = lipgloss.Green
		def.Program = lipgloss.Green
		return def
	})
	return fang.Execute(context.Background(), rootCmd, themeFunc, fang.WithVersion(version.Version))
}

func init() {
	var loggerFile *os.File
	_, debug := os.LookupEnv("DEBUG")

	if debug {
		var fileErr error
		newConfigFile, fileErr := os.OpenFile("debug.log",
			os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
		if fileErr == nil {
			log.SetColorProfile(colorprofile.TrueColor)
			log.SetOutput(newConfigFile)
			log.SetTimeFormat("15:04:05.000")
			log.SetReportCaller(true)
			setDebugLogLevel()
			log.Debug("Logging to debug.log")
		} else {
			loggerFile, _ = tea.LogToFile("debug.log", "debug")
			slog.Print("Failed setting up logging", fileErr)
		}
	} else {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.FatalLevel)
	}

	if loggerFile != nil {
		defer loggerFile.Close()
	}

	rootCmd.SetVersionTemplate(
		logoWithTagline + "\n\n" + `gl-pipeline {{printf "version %s\n" .Version}}`,
	)

	var repo string

	rootCmd.PersistentFlags().StringVarP(
		&repo,
		"repo",
		"R",
		"",
		`[HOST/]GROUP/PROJECT   Select another GitLab project`,
	)

	rootCmd.Flags().String(
		"pipeline",
		"",
		"view a pipeline by its numeric ID",
	)

	rootCmd.Flags().String(
		"theme",
		"",
		"color theme (bubbletint id), e.g. gruvbox_dark, gruvbox_dark_hard, tokyo_night_storm, dracula, nord",
	)

	rootCmd.Flags().Bool(
		"debug",
		false,
		"passing this flag will allow writing debug output to debug.log",
	)

	rootCmd.Flags().BoolP(
		"help",
		"h",
		false,
		"help for gl-pipeline",
	)

	usage := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().
			Bold(true).
			Render("Usage:")+
			" `"+
			lipgloss.NewStyle().
				Foreground(lipgloss.Green).
				Render("gl-pipeline")+
			" https://gitlab.com/group/project/-/merge_requests/42`.",
		"Run "+
			lipgloss.NewStyle().
				Background(lipgloss.Color("#141417")).
				Render("`gl-pipeline --help`")+
			" for help and examples.\n")

	rootCmd.RunE = func(_ *cobra.Command, args []string) error {
		var mrIID int
		var pipelineID int64
		var ref string

		pipelineFlag, _ := rootCmd.Flags().GetString("pipeline")
		if pipelineFlag != "" {
			id, err := strconv.ParseInt(pipelineFlag, 10, 64)
			if err != nil {
				fmt.Print(usage)
				return errors.New("pipeline ID is not a number")
			}
			pipelineID = id
		}

		if len(args) > 0 {
			arg := args[0]
			if strings.Contains(arg, "://") {
				proj, mr, pid, ok := parseGitLabURL(arg)
				if !ok {
					fmt.Print(usage)
					return errors.New("could not parse GitLab URL")
				}
				if repo == "" {
					repo = proj
				}
				if pid > 0 {
					pipelineID = pid
				}
				if mr > 0 {
					mrIID = mr
				}
			} else {
				n, err := strconv.Atoi(arg)
				if err != nil {
					fmt.Print(usage)
					return errors.New("MR IID is not a number")
				}
				mrIID = n
			}
		}

		if repo == "" {
			repo = detectProject()
		}
		if repo == "" {
			fmt.Print(usage)
			return errors.New("could not determine project; use -R group/project")
		}

		// No explicit target: show the latest pipeline for the current branch.
		if pipelineID == 0 && mrIID == 0 {
			ref = currentBranch()
		}

		theme, _ := rootCmd.Flags().GetString("theme")
		if theme != "" && !tui.IsValidTheme(theme) {
			fmt.Printf(
				"unknown theme %q — examples: gruvbox_dark, gruvbox_dark_hard, "+
					"tokyo_night_storm, dracula, nord\n",
				theme,
			)
			return errors.New("unknown theme")
		}

		opts := tui.ModelOpts{
			MRIID:      mrIID,
			PipelineID: pipelineID,
			Ref:        ref,
			Theme:      theme,
		}

		p := tea.NewProgram(tui.NewModel(repo, "", opts))
		if _, err := p.Run(); err != nil {
			log.Error("failed starting program", "err", err)
			fmt.Println(err)
			os.Exit(1)
		}
		return nil
	}
}

// parseGitLabURL extracts the project path and (optionally) an MR IID or
// pipeline ID from a GitLab web URL such as
// https://gitlab.com/group/sub/project/-/merge_requests/42
func parseGitLabURL(raw string) (project string, mrIID int, pipelineID int64, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", 0, 0, false
	}
	path := strings.Trim(u.Path, "/")
	idx := strings.Index(path, "/-/")
	if idx == -1 {
		// bare project URL
		return path, 0, 0, path != ""
	}
	project = path[:idx]
	rest := strings.Split(path[idx+len("/-/"):], "/")
	if len(rest) >= 2 {
		switch rest[0] {
		case "merge_requests":
			mrIID, _ = strconv.Atoi(rest[1])
		case "pipelines":
			pipelineID, _ = strconv.ParseInt(rest[1], 10, 64)
		}
	}
	return project, mrIID, pipelineID, project != ""
}

// detectProject resolves the GitLab project path from the origin remote.
func detectProject() string {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(out))
	raw = strings.TrimSuffix(raw, ".git")

	// scp-like syntax: git@gitlab.com:group/project
	if strings.HasPrefix(raw, "git@") || (!strings.Contains(raw, "://") && strings.Contains(raw, ":")) {
		if i := strings.Index(raw, ":"); i >= 0 {
			return strings.TrimPrefix(raw[i+1:], "/")
		}
	}
	if u, err := url.Parse(raw); err == nil {
		return strings.Trim(u.Path, "/")
	}
	return ""
}

func currentBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func setDebugLogLevel() {
	switch os.Getenv("LOG_LEVEL") {
	case "debug", "":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	}

	log.Debug("log level set", "level", log.GetLevel())
}
