package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"
)

// example branch: ccs-123-hui-blya

// hash = hash()
// branch = git branch --show-current
// git checkout -b <branch>-<hash>  # save branch
// git switch <branch>  # switch to branch
// N = git cherry -v <branch> | wc -l
// git reset --soft HEAD~N && git add . && git commit -m "<branch>" && git push --force

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: gitkick <command>")
		os.Exit(1)
	}

	git := GitService{}
	logger := slog.Default()

	command := os.Args[1]
	switch command {
	case "push":
		if len(os.Args) < 3 {
			fmt.Println("usage: gitkick push <target-branch>")
			os.Exit(1)
		}
		push := PushService{git: &git, logger: logger}
		targetBranch := os.Args[2]
		if err := push.SquashWithForcedPush(targetBranch); err != nil {
			logger.Error("push failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}
}

type GitService struct{}

func (g *GitService) GetCurrentBranchName() (string, error) {
	out, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	branch := strings.TrimSpace(string(out))

	return branch, nil
}

func (p *GitService) generateHash() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	hash := make([]byte, 6)
	for i := range hash {
		hash[i] = charset[r.Intn(len(charset))]
	}

	return string(hash)
}

func (g *GitService) CheckoutNewBranch(branchName string) error {
	if out, err := exec.Command("git", "checkout", "-b", branchName).Output(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, out)
	}
	return nil
}

func (g *GitService) SwitchToBranch(branchName string) error {
	if out, err := exec.Command("git", "switch", branchName).Output(); err != nil {
		return fmt.Errorf("failed to switch branch %s: %w", branchName, out)
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

func (g *GitService) PushForce() error {
	arg := []string{"push", "--force"}
	out, err := exec.Command("git", arg...).Output()
	if err != nil {
		return fmt.Errorf("failed to force push %s", out)
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

type PushService struct {
	logger *slog.Logger
	git    *GitService
}

func (p *PushService) SquashWithForcedPush(targetBranch string) error {
	branch, err := p.git.GetCurrentBranchName()
	if err != nil {
		return err
	}
	p.logger.Info("PushService", "current branch", branch)

	hash := p.git.generateHash()
	newBranch := fmt.Sprintf("%s-%s", branch, hash)
	err = p.git.CheckoutNewBranch(newBranch)
	if err != nil {
		return err
	}
	p.logger.Info("PushService", "current branch saved", newBranch)

	err = p.git.SwitchToBranch(branch)
	if err != nil {
		return err
	}
	p.logger.Info("PushService", "switched to branch", branch)

	diff, err := p.git.GetCommitsDiffCount(targetBranch)
	if err != nil {
		return err
	}
	p.logger.Info("PushService", "diff count", diff, "target_branch", targetBranch)

	err = p.git.ResetSoft(diff)
	if err != nil {
		return err
	}
	p.logger.Info("PushService", "commits reset", diff)

	err = p.git.AddAll()
	if err != nil {
		return nil
	}
	p.logger.Info("PushService", "add all changes", "success")

	err = p.git.Commit(branch)
	if err != nil {
		return err
	}
	p.logger.Info("PushService", "committed", branch)

	err = p.git.PushForce()
	if err != nil {
		return err
	}
	p.logger.Info("PushService", "forced push successful", branch)

	return nil
}
