package sub

import "github.com/mhsanaei/3x-ui/v3/internal/util/clashmerge"

func marshalClashYAML(config any) ([]byte, error) {
	return clashmerge.MarshalYAML(config)
}
