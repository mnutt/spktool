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

type InstallSkillsRequest struct {
	Codex          bool `json:"codex"`
	Claude         bool `json:"claude"`
	Force          bool `json:"force"`
	NonInteractive bool `json:"nonInteractive"`
}

type InstallSkillsResult struct {
	Targets          []string `json:"targets"`
	Files            []string `json:"files"`
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
	for _, target := range targets {
		switch target {
		case codexSkillTarget:
			files, err := s.installCodexSkill(workDir, req.Force)
			if err != nil {
				return nil, err
			}
			result.Files = append(result.Files, files...)
			updated, err := s.ensureGitignoreRule(workDir, codexSkillIgnoreDir)
			if err != nil {
				return nil, err
			}
			result.GitignoreUpdated = result.GitignoreUpdated || updated
		case claudeSkillTarget:
			files, err := s.installClaudeSkill(workDir, req.Force)
			if err != nil {
				return nil, err
			}
			result.Files = append(result.Files, files...)
			updated, err := s.ensureGitignoreRule(workDir, claudeSkillIgnoreDir)
			if err != nil {
				return nil, err
			}
			result.GitignoreUpdated = result.GitignoreUpdated || updated
		default:
			return nil, &domain.Error{Code: domain.ErrInvalidArgument, Op: "services.InstallSkills", Message: fmt.Sprintf("unsupported skill target %q", target)}
		}
	}
	return result, nil
}

func (s *SkillService) installCodexSkill(workDir string, force bool) ([]string, error) {
	paths, err := s.deps.templates.SkillFiles(skillName)
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.installCodexSkill", "list embedded skill files", err)
	}
	files := make([]providers.RenderedFile, 0, len(paths))
	for _, rel := range paths {
		body, err := s.deps.templates.SkillFile(skillName, rel)
		if err != nil {
			return nil, domain.Wrap(domain.ErrExternal, "services.installCodexSkill", "read embedded skill file", err)
		}
		dest := filepath.Join(".codex", "skills", skillName, filepath.FromSlash(rel))
		full := filepath.Join(workDir, dest)
		if !force {
			if _, err := os.Stat(full); err == nil {
				return nil, &domain.Error{
					Code:    domain.ErrConflict,
					Op:      "services.installCodexSkill",
					Message: fmt.Sprintf("%s already exists; rerun with --force to overwrite", filepath.ToSlash(dest)),
				}
			} else if !errorsIsNotExist(err) {
				return nil, domain.Wrap(domain.ErrExternal, "services.installCodexSkill", "read existing skill file", err)
			}
		}
		files = append(files, providers.RenderedFile{
			Path: dest,
			Body: body,
			Mode: 0o644,
		})
	}
	if err := s.deps.writeFiles(workDir, files); err != nil {
		return nil, err
	}

	written := make([]string, 0, len(files))
	for _, file := range files {
		written = append(written, filepath.ToSlash(file.Path))
	}
	return written, nil
}

func (s *SkillService) installClaudeSkill(workDir string, force bool) ([]string, error) {
	paths, err := s.deps.templates.SkillFiles(skillName)
	if err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "services.installClaudeSkill", "list embedded skill files", err)
	}
	files := make([]providers.RenderedFile, 0, len(paths))
	for _, rel := range paths {
		body, err := s.deps.templates.SkillFile(skillName, rel)
		if err != nil {
			return nil, domain.Wrap(domain.ErrExternal, "services.installClaudeSkill", "read embedded skill file", err)
		}
		dest := filepath.Join(".claude", "skills", skillName, filepath.FromSlash(rel))
		full := filepath.Join(workDir, dest)
		if !force {
			if _, err := os.Stat(full); err == nil {
				return nil, &domain.Error{
					Code:    domain.ErrConflict,
					Op:      "services.installClaudeSkill",
					Message: fmt.Sprintf("%s already exists; rerun with --force to overwrite", filepath.ToSlash(dest)),
				}
			} else if !errorsIsNotExist(err) {
				return nil, domain.Wrap(domain.ErrExternal, "services.installClaudeSkill", "read existing skill file", err)
			}
		}
		files = append(files, providers.RenderedFile{
			Path: dest,
			Body: body,
			Mode: 0o644,
		})
	}
	if err := s.deps.writeFiles(workDir, files); err != nil {
		return nil, err
	}
	written := make([]string, 0, len(files))
	for _, file := range files {
		written = append(written, filepath.ToSlash(file.Path))
	}
	return written, nil
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
