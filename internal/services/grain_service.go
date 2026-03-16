package services

import (
	"context"
	"crypto/sha1"
	"fmt"
	"strings"

	"github.com/mnutt/spktool/internal/domain"
)

func (s *GrainService) EnterGrain(ctx context.Context, workDir string, providerOverride domain.ProviderName, noninteractive bool) (*domain.ProjectState, error) {
	projectState, resolved, plugin, err := s.deps.loadGrainProject(ctx, workDir, providerOverride)
	if err != nil {
		return nil, err
	}
	helper, err := s.deps.templates.HelperFile("enter_grain")
	if err != nil {
		return nil, err
	}
	checksumFile, err := s.deps.templates.HelperFile("enter_grain.sha1")
	if err != nil {
		return nil, err
	}
	desiredChecksum := ""
	if fields := strings.Fields(string(checksumFile)); len(fields) > 0 {
		desiredChecksum = strings.TrimSpace(fields[0])
	}
	if desiredChecksum != "" {
		found := fmt.Sprintf("%x", sha1.Sum(helper))
		if found != desiredChecksum {
			return nil, &domain.Error{Code: domain.ErrConflict, Op: "services.EnterGrain", Message: "embedded enter_grain helper checksum mismatch"}
		}
	}
	project := s.deps.projectContext(workDir, projectState, resolved)
	grains, err := plugin.ListGrains(ctx, project)
	if err != nil {
		return nil, err
	}
	chosen, err := chooseGrain(grains, noninteractive)
	if err != nil {
		return nil, err
	}
	if err := plugin.AttachGrain(ctx, project, chosen, helper, desiredChecksum); err != nil {
		return nil, err
	}
	return projectState, nil
}
