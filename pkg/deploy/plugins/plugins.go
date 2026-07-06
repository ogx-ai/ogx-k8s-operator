package plugins

import (
	"fmt"
	"strings"

	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
)

// ValidateK8sLabelName uses the strictest naming rule (for Namespaces) to ensure
// the provided name string is valid for any given Resource.
func ValidateK8sLabelName(name string) error {
	if errs := k8svalidation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("failed to validate name %q: %s", name, strings.Join(errs, ", "))
	}
	return nil
}
