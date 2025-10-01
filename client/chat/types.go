package chat

import (
	core "github.com/viant/agently/genai/service/core"
	internal "github.com/viant/agently/internal/service/chat"
)

type (
	GetRequest                 = internal.GetRequest
	GetResponse                = internal.GetResponse
	PostRequest                = internal.PostRequest
	CreateConversationRequest  = internal.CreateConversationRequest
	CreateConversationResponse = internal.CreateConversationResponse
	ConversationSummary        = internal.ConversationSummary
	GenerateInput              = core.GenerateInput
	GenerateOutput             = core.GenerateOutput
)
