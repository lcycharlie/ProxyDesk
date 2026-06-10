package uistate

import (
	"strconv"
	"strings"

	"proxydesk/internal/routing"
)

type PortRangeText struct {
	Start string
	End   string
}

func PortRangeFromText(input PortRangeText) routing.PortRange {
	portRange := routing.PortRange{
		Start: routing.DefaultPortStart,
		End:   routing.DefaultPortEnd,
	}
	if value, err := strconv.Atoi(strings.TrimSpace(input.Start)); err == nil {
		portRange.Start = value
	}
	if value, err := strconv.Atoi(strings.TrimSpace(input.End)); err == nil {
		portRange.End = value
	}
	return portRange
}

func ValidatePortRangeText(input PortRangeText) (routing.PortRange, error) {
	portRange := PortRangeFromText(input)
	if err := routing.ValidatePortRange(portRange); err != nil {
		return routing.PortRange{}, err
	}
	return portRange, nil
}

func AvailablePortOptions(input PortRangeText, usedPorts map[int]bool) []string {
	portRange := PortRangeFromText(input)
	if portRange.Start > portRange.End {
		return []string{}
	}
	return routing.PortOptions(portRange, usedPorts)
}

func PreferredPortSelection(options []string, keepPort int) (text string, index int) {
	if keepPort > 0 {
		return strconv.Itoa(keepPort), -1
	}
	if len(options) > 0 {
		return "", 0
	}
	return "", -1
}
