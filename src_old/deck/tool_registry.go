package deck

import (
	"fmt"
	"os/exec"

	"argo/src_old/knot"
)

// validateCLIEntry 校验单个 CLI 工具条目，name 即 executable
func validateCLIEntry(name string, e knot.CLIToolEntry) []error {
	var errs []error

	if name == "" {
		errs = append(errs, fmt.Errorf("missing name"))
	}

	if _, err := exec.LookPath(name); err != nil {
		errs = append(errs, fmt.Errorf("binary not found"))
	}

	if e.Description == "" {
		errs = append(errs, fmt.Errorf("description empty"))
	}

	return errs
}

// ValidateConfig 校验全部 CLI 工具条目，收集所有错误后一并返回
func ValidateConfig(cfg knot.DeckConfig) []error {
	var errs []error

	for name, e := range cfg.CLI {
		for _, err := range validateCLIEntry(name, e) {
			errs = append(errs, fmt.Errorf("cli.%s: %w", name, err))
		}
	}

	return errs
}
