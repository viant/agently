package tool

import (
	"reflect"

	"github.com/viant/agently/genai/tool"
	"github.com/viant/xdatly/types/core"
	"github.com/viant/xdatly/types/custom/dependency/checksum"
)

func init() {
	// Backward-compatible registrations under "extension"
	core.RegisterType("tool", "Feed", reflect.TypeOf(tool.Feed{}), checksum.GeneratedTime)
	core.RegisterType("tool", "FeedSpec", reflect.TypeOf(tool.FeedSpec{}), checksum.GeneratedTime)
	// New naming under "feed"
}

type Feed = tool.Feed
type FeedSpec = tool.FeedSpec
