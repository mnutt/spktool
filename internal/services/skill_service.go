package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/providers"
)

const (
	skillName            = "sandstorm-app-author"
	codexSkillTarget     = "codex"
	codexSkillIgnoreDir  = ".codex/skills/sandstorm-app-author/"
	claudeSkillTarget    = "claude"
	claudeSkillIgnoreDir = ".claude/skills/sandstorm-app-author/"
)

type skillInstallTarget struct {
	name      string
	dir       string
	ignoreDir string
	op        string
}

var skillTargets = map[string]skillInstallTarget{
	codexSkillTarget: {
		name:      codexSkillTarget,
		dir:       filepath.Join(".codex", "skills", skillName),
		ignoreDir: codexSkillIgnoreDir,
		op:        "services.installCodexSkill",
	},
	claudeSkillTarget: {
		name:      claudeSkillTarget,
		dir:       filepath.Join(".claude", "skills", skillName),
		ignoreDir: claudeSkillIgnoreDir,
		op:        "services.installClaudeSkill",
	},
}

type InstallSkillsRequest struct {
	Codex          bool `json:"codex"`
	Claude         bool `json:"claude"`
	Force          bool `json:"force"`
	NonInteractive bool `json:"nonInteractive"`
}

type InstallSkillsResult struct {
	Targets          []string `json:"targets"`
	Directories      []string `json:"directories"`
	GitignoreUpdated bool     `json:"gitignoreUpdated"`
}

func (s *SkillService) InstallSkills(_ context.Context, workDir string, req InstallSkillsRequest) (*InstallSkillsResult, error) {
	targets := make([]string, 0, 2)
	if req.Codex {
		targets = append(targets, codexSkillTarget)
	}
	if req.Claude {
		targets = append(targets, claudeSkillTarget)
	}
	if len(targets) == 0 {
		if s.deps.hasOnPath("codex") {
			targets = append(targets, codexSkillTarget)
		}
		if s.deps.hasOnPath("claude") {
			targets = append(targets, claudeSkillTarget)
		}
	}
	if len(targets) == 0 {
		msg := "no supported skill targets detected"
		if req.NonInteractive {
			msg += "; rerun with install-skills --codex and/or --claude"
		} else {
			msg += "; install Codex or Claude Code, or rerun with install-skills --codex and/or --claude"
		}
		return nil, &domain.Error{Code: domain.ErrNotFound, Op: "services.InstallSkills", Message: msg}
	}

	result := &InstallSkillsResult{
		Targets: append([]string(nil), targets...),
	}
	for _, name := range targets {
		target, ok := skillTargets[name]
		if !ok {
			return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.InstallSkills", Message: fmt.Sprintf("unsupported skill target %q", name)}
		}
		if err := s.installSkillTarget(workDir, target, req.Force); err != nil {
			return nil, err
		}
		result.Directories = append(result.Directories, filepath.ToSlash(target.dir)+"/")
		updated, err := s.ensureGitignoreRule(workDir, target.ignoreDir)
		if err != nil {
			return nil, err
		}
		result.GitignoreUpdated = result.GitignoreUpdated || updated
	}
	return result, nil
}

func (s *SkillService) installSkillTarget(workDir string, target skillInstallTarget, force bool) error {
	paths, err := s.deps.templates.SkillFiles(skillName)
	if err != nil {
		return domain.Wrap(domain.ErrExternal, target.op, "list embedded skill files", err)
	}
	files := make([]providers.RenderedFile, 0, len(paths))
	for _, rel := range paths {
		body, err := s.deps.templates.SkillFile(skillName, rel)
		if err != nil {
			return domain.Wrap(domain.ErrExternal, target.op, "read embedded skill file", err)
		}
		dest := filepath.Join(target.dir, filepath.FromSlash(rel))
		full := filepath.Join(workDir, dest)
		if !force {
			if _, err := os.Stat(full); err == nil {
				return &domain.Error{
					Code:    domain.ErrConflict,
					Op:      target.op,
					Message: fmt.Sprintf("%s already exists; rerun with --force to overwrite", filepath.ToSlash(dest)),
				}
			} else if !errorsIsNotExist(err) {
				return domain.Wrap(domain.ErrExternal, target.op, "read existing skill file", err)
			}
		}
		files = append(files, providers.RenderedFile{
			Path: dest,
			Body: body,
			Mode: 0o644,
		})
	}
	if err := s.deps.writeFiles(workDir, files); err != nil {
		return err
	}
	return nil
}

func (s *SkillService) ensureGitignoreRule(workDir, rule string) (bool, error) {
	path := filepath.Join(workDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errorsIsNotExist(err) {
		return false, domain.Wrap(domain.ErrExternal, "services.ensureGitignoreRule", "read .gitignore", err)
	}
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == rule {
				return false, nil
			}
		}
	}

	body := rule + "\n"
	if err == nil && len(data) > 0 {
		body = string(data)
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += rule + "\n"
	}
	if writeErr := os.WriteFile(path, []byte(body), 0o644); writeErr != nil {
		return false, domain.Wrap(domain.ErrExternal, "services.ensureGitignoreRule", "write .gitignore", writeErr)
	}
	return true, nil
}

func defaultLookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}
