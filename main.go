package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"context"
	"io"
	"sync"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: kk <command> [flags]")
		os.Exit(1)
	}

	git := GitService{}
	logger := slog.New(NewColorHandler(os.Stderr, slog.LevelInfo))
	manage := CodeFlowManageService{git: &git, logger: logger}

	command := os.Args[1]
	switch command {
	case "squash":
		comparableBranch := parseFlag(os.Args[2:], "--compare")
		if comparableBranch == "" {
			comparableBranch = "develop"
		}
		message := parseFlag(os.Args[2:], "--message")
		if err := manage.Squash(comparableBranch, message); err != nil {
			logger.Error("squash failed", "error", err)
			os.Exit(1)
		}
	case "clean":
		if err := manage.CleanFallbackBranches(); err != nil {
			logger.Error("clean failed", "error", err)
			os.Exit(1)
		}
	case "commit":
		if err := manage.Commit(); err != nil {
			logger.Error("commit failed", "error", err)
			os.Exit(1)
		}
	case "push":
		if err := manage.Push(); err != nil {
			logger.Error("push failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}
}

func parseFlag(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

var branchRegex = regexp.MustCompile(`^([A-Za-z]+)[/-](\d+)-(.+)$`)

func commitMessageFromBranch(branch string) string {
	matches := branchRegex.FindStringSubmatch(branch)
	if matches == nil {
		return branch
	}
	prefix := matches[1]
	number := matches[2]
	description := strings.ReplaceAll(matches[3], "-", " ")
	return fmt.Sprintf("[%s-%s] %s", prefix, number, description)
}

// --------------------------------- Git ---------------------------------------

type GitService struct{}

func (g *GitService) GetCurrentBranchName() (string, error) {
	out, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	branch := strings.TrimSpace(string(out))

	return branch, nil
}

func (p *GitService) generateTimestamp() string {
	return time.Now().Format("2006-01-02-15-04-05")
}

func (g *GitService) NewBranch(branchName string) error {
	if out, err := exec.Command("git", "branch", branchName).Output(); err != nil {
		return fmt.Errorf("failed to create branch %s: %s", branchName, out)
	}
	return nil
}

func (g *GitService) SwitchToBranch(branchName string) error {
	if out, err := exec.Command("git", "switch", branchName).Output(); err != nil {
		return fmt.Errorf("failed to switch branch %s: %s", branchName, out)
	}
	return nil
}

func (g *GitService) GetCommitsDiffCount(targetBranch string) (int, error) {
	out, err := exec.Command("git", "cherry", "-v", targetBranch).Output()
	if err != nil {
		return 0, fmt.Errorf("failed to count diff for target branch %s: %w", targetBranch, err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, nil
	}

	lines := strings.Split(trimmed, "\n")
	return len(lines), nil
}

func (g *GitService) Commit(message string) error {
	out, err := exec.Command("git", "commit", "-m", message).Output()
	if err != nil {
		return fmt.Errorf("failed to commit %s", out)
	}
	return err
}

func (g *GitService) Push(branchName string) error {
	arg := []string{"push", "--set-upstream", "origin", branchName}
	out, err := exec.Command("git", arg...).Output()
	if err != nil {
		return fmt.Errorf("failed to push %s", out)
	}
	return err
}

func (g *GitService) ResetSoft(commitsFromHead int) error {
	commitsToReset := fmt.Sprintf("HEAD~%d", commitsFromHead)
	out, err := exec.Command("git", "reset", "--soft", commitsToReset).Output()
	if err != nil {
		return fmt.Errorf("failed to reset softly %s", out)
	}
	return err
}

func (g *GitService) AddAll() error {
	out, err := exec.Command("git", "add", ".").Output()
	if err != nil {
		return fmt.Errorf("failed to add all changes %s", out)
	}
	return err
}

func (g *GitService) DeleteLocalBranch(branchName string) error {
	out, err := exec.Command("git", "branch", "-D", branchName).Output()
	if err != nil {
		return fmt.Errorf("failed to delete branch %s err %s", branchName, out)
	}
	return err
}

func (g *GitService) StatusWithPorcelain() (string, error) {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return "", fmt.Errorf("failed to check working tree status: %s", out)
	}
	return string(out), nil
}

func (g *GitService) ListBranchesWithPrefix(prefix string) ([]string, error) {
	out, err := exec.Command("git", "branch", "--list", prefix+"*").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	var branches []string
	for _, line := range strings.Split(trimmed, "\n") {
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

// --------------------------------- Services ---------------------------------------

type CodeFlowManageService struct {
	logger *slog.Logger
	git    *GitService
}

func (s *CodeFlowManageService) CleanFallbackBranches() error {
	branches, err := s.git.ListBranchesWithPrefix("kk-fallback")
	if err != nil {
		return err
	}

	if len(branches) == 0 {
		s.logger.Info("Clean", "message", "no fallback branches found")
		return nil
	}

	for _, branch := range branches {
		if err := s.git.DeleteLocalBranch(branch); err != nil {
			return err
		}
		s.logger.Info("Clean", "deleted", branch)
	}

	s.logger.Info("Clean", "total deleted", len(branches))
	return nil
}

func (s *CodeFlowManageService) Commit() error {
	branch, err := s.git.GetCurrentBranchName()
	if err != nil {
		return err
	}

	if err := s.git.AddAll(); err != nil {
		return err
	}
	s.logger.Info("Commit", "status", "staged all changes")

	message := commitMessageFromBranch(branch)
	if err := s.git.Commit(message); err != nil {
		return err
	}
	s.logger.Info("Commit", "committed with message", message)

	return nil
}

func (s *CodeFlowManageService) Push() error {
	out, err := s.git.StatusWithPorcelain()
	if err != nil {
		return err
	}
	if out != "" {
		if err := s.Commit(); err != nil {
			return err
		}
	} else {
		s.logger.Info("Push", "no changes to commit", "skip commit")
	}

	branch, err := s.git.GetCurrentBranchName()
	if err != nil {
		return err
	}

	if err := s.git.Push(branch); err != nil {
		return err
	}
	s.logger.Info("Push", "pushed branch", branch)

	return nil
}

func (s *CodeFlowManageService) Squash(comparableBranch string, commitMessage string) error {
	status, err := s.git.StatusWithPorcelain()
	if err != nil {
		return fmt.Errorf("failed to get working tree status: %s", err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("working tree is not clean: %s", status)
	}

	currentBranch, err := s.git.GetCurrentBranchName()
	if err != nil {
		return err
	}
	s.logger.Info("Squash", "current branch", currentBranch)

	if comparableBranch == currentBranch {
		return fmt.Errorf("comparable branch is the same as current branch")
	}

	diff, err := s.git.GetCommitsDiffCount(comparableBranch)
	if err != nil {
		return err
	}
	s.logger.Info("Squash", "diff count", diff, "between", currentBranch, "and", comparableBranch)

	if diff <= 1 {
		s.logger.Info("Squash", "diff <= 1", "nothing to squash")
		return nil
	}

	ts := s.git.generateTimestamp()
	fallbackBranch := fmt.Sprintf("%s-%s-%s", "kk-fallback", currentBranch, ts)
	err = s.git.NewBranch(fallbackBranch)
	if err != nil {
		return err
	}
	s.logger.Info("Squash", "fallback branch", fallbackBranch)

	err = s.git.ResetSoft(diff)
	if err != nil {
		return err
	}
	s.logger.Info("Squash", "commits reset", diff, "on branch", currentBranch)

	err = s.git.AddAll()
	if err != nil {
		return nil
	}
	s.logger.Info("Squash", "add all changes on branch", currentBranch)

	message := commitMessage
	if message == "" {
		message = commitMessageFromBranch(currentBranch)
	}
	err = s.git.Commit(message)
	if err != nil {
		return err
	}
	s.logger.Info("Squash", "squash committed as", message, "on branch", currentBranch)

	return nil
}

// --------------------------------- Custom log colloring ---------------------------------

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

type ColorHandler struct {
	out   io.Writer
	mu    *sync.Mutex
	level slog.Level
}

func NewColorHandler(out io.Writer, level slog.Level) *ColorHandler {
	return &ColorHandler{
		out:   out,
		mu:    &sync.Mutex{},
		level: level,
	}
}

func (h *ColorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *ColorHandler) Handle(_ context.Context, r slog.Record) error {
	levelColor := colorCyan
	switch {
	case r.Level >= slog.LevelError:
		levelColor = colorRed
	case r.Level >= slog.LevelWarn:
		levelColor = colorYellow
	case r.Level >= slog.LevelInfo:
		levelColor = colorGreen
	}

	timeStr := r.Time.Format(time.Kitchen)

	msg := fmt.Sprintf("%s%s%s %s%-5s%s %s",
		colorGray, timeStr, colorReset,
		levelColor, r.Level.String(), colorReset,
		r.Message,
	)

	r.Attrs(func(a slog.Attr) bool {
		msg += fmt.Sprintf(" %s%s%s %v", colorCyan, a.Key, colorReset, a.Value)
		return true
	})

	msg += "\n"

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprint(h.out, msg)
	return err
}

func (h *ColorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *ColorHandler) WithGroup(name string) slog.Handler {
	return h
}
