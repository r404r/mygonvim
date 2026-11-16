package nvimproc

import (
	"errors"
	"strconv"
	"strings"

	"github.com/neovim/go-client/nvim"
)

func clientVersion(version string) nvim.ClientVersion {
	if version == "" || version == "dev" {
		return nvim.ClientVersion{Prerelease: "dev"}
	}
	base, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), "+")
	base, pre, _ := strings.Cut(base, "-")
	parts := strings.Split(base, ".")
	var numbers [3]int
	if len(parts) != 3 {
		return nvim.ClientVersion{Prerelease: version}
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nvim.ClientVersion{Prerelease: version}
		}
		numbers[i] = n
	}
	return nvim.ClientVersion{Major: numbers[0], Minor: numbers[1], Patch: numbers[2], Prerelease: pre}
}

func (s *Session) identify(version string) error {
	// ChannelID obtains nvim_get_api_info before identification and UI attach.
	if s.nv.ChannelID() == 0 {
		return errors.New("nvim_get_api_info: missing channel ID")
	}
	methods := map[string]*nvim.ClientMethod{}
	if s.clipboard != nil {
		methods["mygonvim.clipboard_get"] = &nvim.ClientMethod{NArgs: nvim.ClientMethodNArgs{Min: 1, Max: 1}}
		methods["mygonvim.clipboard_set"] = &nvim.ClientMethod{NArgs: nvim.ClientMethodNArgs{Min: 3, Max: 3}}
	}
	return s.nv.SetClientInfo("mygonvim", clientVersion(version), nvim.UIClientType,
		methods, nvim.ClientAttributes{
			"website": "https://github.com/r404r/mygonvim", "license": "MIT",
		})
}

// registerErrors is shared with the raw RPC transport used to verify actual
// notification errors from Neovim. The callback must never make a blocking RPC.
func (s *Session) registerErrors(register func(string, any) error) error {
	return register("nvim_error_event", s.errorEvent)
}

func (s *Session) errorEvent(kind int, message string) {
	s.callMu.Lock()
	recent := s.recentCall
	s.callMu.Unlock()
	s.logf("nvim_error_event: type=%d message=%q recent_call_hint=%q (not a request ID)", kind, message, recent)
}

func (s *Session) calling(method string) {
	s.callMu.Lock()
	s.recentCall = method
	s.callMu.Unlock()
}
