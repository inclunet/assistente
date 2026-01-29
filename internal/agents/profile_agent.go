package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/database"
)

// ProfileAgent is an intelligent agent that manages Voice and Interaction profiles
type ProfileAgent struct {
	BaseAgent
	activateCallback                 func(profileID uint) error                 // Register hotkeys when activating
	deactivateCallback               func(profileID uint) error                 // Unregister hotkeys when deactivating
	emitEventCallback                func(event string, data interface{})       // Emit Wails events
	applyVoiceToConversationCallback func(conversationID, profileID uint) error // Apply voice profile to conversation
}

// profileAgentDescription returns structured description for orchestrator delegation
func profileAgentDescription() string {
	return NewDelegationDescription("Profile Manager", "Specialist in managing Voice (TTS) and Interaction (STT) profiles").
		Capabilities(
			"Create, update, delete Voice Profiles (text-to-speech configuration)",
			"Configure TTS provider (OpenAI, WebSpeech, SAPI5), voice, speed, volume",
			"Apply voice profile to the current conversation",
			"Create, update, delete Interaction Profiles (speech-to-text configuration)",
			"Configure STT triggers: hotkeys, wakewords, push-to-talk, VAD",
			"Set default profiles and activate interaction profiles",
			"List available voices for each provider",
		).
		DelegateWhen(
			"User wants to configure how the assistant speaks (TTS/voice)",
			"User wants to apply a voice profile to this conversation",
			"User wants to configure how to interact by voice (STT/microphone)",
			"User mentions 'voice profile', 'speech settings', 'TTS', 'text-to-speech'",
			"User mentions 'interaction profile', 'hotkey', 'wakeword', 'push-to-talk', 'VAD'",
			"User wants to change voice speed, volume, or voice selection",
			"User wants to set up keyboard shortcuts for voice interaction",
			"User says 'activate voice', 'use OpenAI voice', 'speak with this voice'",
		).
		DontDelegateWhen(
			"User just wants to use voice features (not configure them)",
			"Questions about general settings unrelated to voice",
			"User wants to configure LLM models or API keys",
		).
		Build()
}

// profileAgentSystemPrompt returns the system prompt for the agent
func profileAgentSystemPrompt() string {
	return `You are the Profile Manager, specialist in configuring Voice and Interaction profiles.

## YOUR TWO DOMAINS

### DOMAIN 1: Voice Profiles (TTS - How the ASSISTANT speaks)
Controls text-to-speech synthesis for assistant responses.
- Provider: openai (high quality), webspeech (free), sapi5 (Windows offline), disabled
- Voice selection, speed (rate), pitch, volume
- Whether to auto-read assistant responses (enabled_for_agent)
- Whether to read user messages aloud (enabled_for_user)
Tools: voice_profile_*

### DOMAIN 2: Interaction Profiles (STT - How the USER speaks)
Controls speech recognition and activation methods.
- STT Provider: webspeech (free, real-time) or whisper_api (high accuracy)
- Language setting for recognition
- Triggers: how to start/stop voice recording
Tools: interaction_profile_*

## CRITICAL RULE: NEVER ASSUME INFORMATION
- ALWAYS list existing profiles BEFORE creating new ones
- If essential information is missing: ASK the user
- NEVER invent values for parameters the user didn't provide
- When in doubt, present available options and ask for preference

## VOICE PROFILE CREATION FLOW
1. Ask or confirm:
   - Which provider? (OpenAI = paid high quality, WebSpeech = free, SAPI5 = Windows offline)
   - Which voice? (depends on provider)
   - Speed different from default? (1.0 = normal, 0.5 = slow, 2.0 = fast)
   - Should assistant auto-read its responses?
2. Only execute voice_profile_create when you have ALL information

## APPLYING VOICE PROFILE TO CONVERSATION
When user wants to USE a voice profile for the CURRENT conversation:
1. First, list existing profiles with voice_profile_list to find the profile ID
2. Use voice_profile_apply_to_conversation with the profile_id
3. This ONLY affects the current conversation, not other conversations

IMPORTANT: If user says "activate OpenAI voice" or "use voice X for this chat":
- This means APPLY to current conversation, NOT create a new profile
- Use voice_profile_apply_to_conversation, NOT voice_profile_create

## MODIFYING AN EXISTING VOICE PROFILE
When user wants to CHANGE settings of an existing profile (speed, volume, enabled_for_user, etc.):
1. First, use voice_profile_get to see current settings of the profile
2. Use voice_profile_update with the profile_id and the fields to change
3. After updating, if this profile is applied to current conversation, use voice_profile_apply_to_conversation to refresh

IMPORTANT: If user says "disable voice for my messages" or "change the speed":
- This means UPDATE the profile settings, use voice_profile_update
- Pass only the fields that need to change (e.g., enabled_for_user: false)
- Then re-apply the profile to the conversation if needed

Examples of UPDATE requests:
- "disable voice for my messages" → voice_profile_update with enabled_for_user=false
- "make it faster" → voice_profile_update with rate=1.5
- "lower the volume" → voice_profile_update with volume=0.5
- "only read your responses" → voice_profile_update with enabled_for_user=false, enabled_for_agent=true

## INTERACTION PROFILE CREATION FLOW
1. Ask or confirm:
   - How should recording be activated?
     * Hotkey (keyboard shortcut) → which combination?
     * Push-to-talk button or toggle button?
     * Wakeword (activation word) → which word?
     * VAD (automatic voice detection)?
   - Which STT provider? (WebSpeech = free, Whisper API = better quality)
   - Which language?
2. For each trigger, confirm specific details
3. Only execute interaction_profile_create when you have ALL information

## WHEN TO ASK
- User says "create a voice profile" without specifying provider → ASK
- User says "I want a hotkey" without saying which → ASK
- User mentions "fast voice" without specifying speed → SUGGEST values and ASK
- Any ambiguity → PRESENT options and ASK

## HOW TO ASK
Be direct and offer clear options. Example:
"To create the voice profile, I need to know:
1. Which provider? OpenAI (high quality, paid) or WebSpeech (free)?
2. If OpenAI, which voice? nova (female), alloy, echo, fable, onyx, shimmer"

## AVAILABLE VOICES

**OpenAI TTS** (high quality, requires API key):
- nova: female, natural, expressive (most popular)
- alloy: neutral, versatile
- echo: male, clear tone
- fable: expressive, good for narratives
- onyx: male, deep voice
- shimmer: female, soft tone

**WebSpeech** (free, quality varies by browser):
- Depends on browser/OS, use voice_list_available to check

**SAPI5** (Windows, offline):
- Microsoft voices installed on the system
- Use voice_list_available to check

## TRIGGER TYPES FOR INTERACTION PROFILES
- hotkey: Keyboard shortcut (e.g., "Ctrl+Shift+Space")
- button_ptt: Push-to-talk (hold to speak, release to send)
- button_toggle: Toggle mode (click to start, click to stop)
- wakeword: Activation word (e.g., "assistant", "computer")
- vad: Voice Activity Detection (auto-start when speaking, auto-stop on silence)

## CONTEXT AWARENESS
If user asks about "profile" without specifying:
- Ask: "Do you want to configure how the assistant speaks (voice profile) or how you speak to it (interaction profile)?"`
}

// GetDelegationDescription implements DelegationDescriptionProvider
func (a *ProfileAgent) GetDelegationDescription() string {
	return profileAgentDescription()
}

// NewProfileAgent creates a new ProfileAgent
func NewProfileAgent(llmClient LLMClient, model string) *ProfileAgent {
	if model == "" {
		model = "gpt-4o-mini"
	}

	return &ProfileAgent{
		BaseAgent: BaseAgent{
			Name:         "profile",
			DisplayName:  "Profile Manager",
			Description:  "Specialist in managing Voice (TTS) and Interaction (STT) profiles. Configures how the assistant speaks and how the user interacts by voice.",
			AgentType:    "internal",
			Model:        model,
			SystemPrompt: profileAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
		},
	}
}

// SetCallbacks configures the integration callbacks
func (a *ProfileAgent) SetCallbacks(
	activate func(uint) error,
	deactivate func(uint) error,
	emitEvent func(string, interface{}),
	applyVoiceToConversation func(conversationID, profileID uint) error,
) {
	a.activateCallback = activate
	a.deactivateCallback = deactivate
	a.emitEventCallback = emitEvent
	a.applyVoiceToConversationCallback = applyVoiceToConversation
}

// Execute receives a task in natural language and uses the LLM to decide how to solve it
func (a *ProfileAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client not configured for agent %s", a.Name)
	}

	fmt.Printf("👤 [Profile Agent] Received task: %s\n", task)

	var result string
	var err error
	if a.MessageSaver != nil {
		result, err = a.LLM.ChatWithToolsAndSaver(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
			a.Name,
			a.MessageSaver,
		)
	} else {
		result, err = a.LLM.ChatWithTools(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
		)
	}

	if err != nil {
		return "", fmt.Errorf("error in Profile Agent execution: %w", err)
	}

	return result, nil
}

// CanHandle checks if the agent can execute a tool
func (a *ProfileAgent) CanHandle(toolName string) bool {
	return false
}

// GetTools returns the available tools for the agent
func (a *ProfileAgent) GetTools() []Tool {
	return []Tool{
		// ==================== Voice Profile Tools ====================
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_profile_list",
				Description: NewToolDescription("Lists all voice profiles (TTS configurations)").
					WhenToUse(
						"ALWAYS as FIRST STEP before creating any voice profile",
						"To check if a profile for a specific voice already exists",
						"To find the ID of an existing profile",
						"To see which voice configurations are available",
					).
					WhenNotToUse(
						"If already listed recently in the same conversation",
					).
					Returns("List of voice profiles with ID, name, provider, voice, status (default/enabled)").
					Notes(
						"Use this list to avoid creating duplicate profiles",
						"Shows provider, voice_id, rate, and whether it's the default",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"query": JSONSchemaString(
						NewParamDescription("Optional filter by name or description").Build(),
					),
				}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_profile_get",
				Description: NewToolDescription("Gets complete details of a specific voice profile").
					WhenToUse(
						"To see all settings of a specific profile",
						"Before updating, to see current values",
						"When user asks about a specific profile",
					).
					Returns("Full profile details: provider, voice, rate, pitch, volume, enabled flags").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(
						NewParamDescription("Voice profile ID").Build(),
					),
					"name": JSONSchemaString(
						NewParamDescription("Voice profile name (alternative to ID)").Build(),
					),
				}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_profile_create",
				Description: NewToolDescription("Creates a new voice profile for text-to-speech").
					WhenToUse(
						"ONLY when you have ALL required information",
						"After confirming provider and voice with user",
						"After checking no similar profile exists",
					).
					WhenNotToUse(
						"If user didn't specify provider → ASK first",
						"If user didn't choose voice → LIST options and ASK",
						"If there's any ambiguity → CONFIRM before creating",
						"NEVER assume values user didn't provide",
					).
					Returns("Created profile ID and confirmation").
					Notes(
						"Providers: disabled, openai, webspeech, sapi5",
						"OpenAI voices: nova, alloy, echo, fable, onyx, shimmer",
						"rate: 0.5-2.0 (1.0 = normal speed)",
						"volume: 0.0-1.0 (1.0 = full volume)",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"name": JSONSchemaString(
						NewParamDescription("Unique profile name").
							Examples("Main Voice", "Fast Reading", "Quiet Mode").Build(),
					),
					"description": JSONSchemaString(
						NewParamDescription("Profile description").Build(),
					),
					"provider": JSONSchemaStringEnum(
						NewParamDescription("TTS provider").Build(),
						[]string{"disabled", "openai", "webspeech", "sapi5"},
					),
					"voice_id": JSONSchemaString(
						NewParamDescription("Voice identifier").
							Examples("nova", "alloy", "Microsoft Maria").Build(),
					),
					"rate": JSONSchemaNumber(
						NewParamDescription("Speech speed (1.0 = normal)").
							Default("1.0").Build(),
					),
					"pitch": JSONSchemaNumber(
						NewParamDescription("Voice pitch (WebSpeech only, 0.5-2.0)").
							Default("1.0").Build(),
					),
					"volume": JSONSchemaNumber(
						NewParamDescription("Volume (0.0 to 1.0)").
							Default("1.0").Build(),
					),
					"enabled_for_agent": JSONSchemaBool(
						NewParamDescription("Auto-read assistant responses").
							Default("true").Build(),
					),
					"enabled_for_user": JSONSchemaBool(
						NewParamDescription("Read user messages aloud").
							Default("false").Build(),
					),
					"is_default": JSONSchemaBool(
						NewParamDescription("Set as default profile").
							Default("false").Build(),
					),
				}, []string{"name", "provider"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_profile_update",
				Description: NewToolDescription("Updates an existing voice profile").
					WhenToUse(
						"To change voice, speed, volume, or other settings",
						"To enable/disable TTS for agent or user",
						"After confirming changes with user",
					).
					Returns("Update confirmation").
					Notes(
						"Only pass fields you want to change",
						"Use voice_profile_get first to see current values",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id":        JSONSchemaInt(NewParamDescription("Profile ID to update").Build()),
					"name":              JSONSchemaString(NewParamDescription("New name").Build()),
					"description":       JSONSchemaString(NewParamDescription("New description").Build()),
					"provider":          JSONSchemaStringEnum(NewParamDescription("New provider").Build(), []string{"disabled", "openai", "webspeech", "sapi5"}),
					"voice_id":          JSONSchemaString(NewParamDescription("New voice ID").Build()),
					"rate":              JSONSchemaNumber(NewParamDescription("New speech rate").Build()),
					"pitch":             JSONSchemaNumber(NewParamDescription("New pitch").Build()),
					"volume":            JSONSchemaNumber(NewParamDescription("New volume").Build()),
					"enabled_for_agent": JSONSchemaBool(NewParamDescription("Enable for assistant responses").Build()),
					"enabled_for_user":  JSONSchemaBool(NewParamDescription("Enable for user messages").Build()),
					"is_default":        JSONSchemaBool(NewParamDescription("Set as default").Build()),
				}, []string{"profile_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_profile_delete",
				Description: NewToolDescription("Deletes a voice profile").
					WhenToUse(
						"When user confirms deletion",
						"To remove unused profiles",
					).
					WhenNotToUse(
						"Without user confirmation - this is irreversible",
						"If it's the only profile or the default",
					).
					Returns("Deletion confirmation").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(NewParamDescription("Profile ID to delete").Build()),
				}, []string{"profile_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_profile_set_default",
				Description: NewToolDescription("Sets a voice profile as the default").
					WhenToUse(
						"When user wants to change the default voice profile",
						"After creating a new profile user wants as default",
					).
					Returns("Confirmation of new default").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(NewParamDescription("Profile ID to set as default").Build()),
				}, []string{"profile_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_profile_apply_to_conversation",
				Description: NewToolDescription("Applies a voice profile to the current conversation").
					WhenToUse(
						"When user wants to use a specific voice profile for THIS conversation",
						"When user says 'use this voice for this chat' or similar",
						"When user wants to change how the assistant speaks in the current conversation",
					).
					WhenNotToUse(
						"If user wants to change the global default - use voice_profile_set_default instead",
					).
					Returns("Confirmation that profile was applied to the conversation").
					Notes(
						"This only affects the current conversation",
						"Other conversations will use their own profile or the default",
						"Use profile_id=0 to remove the profile (use default instead)",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(
						NewParamDescription("Voice profile ID to apply (0 to use default)").Build(),
					),
				}, []string{"profile_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "voice_list_available",
				Description: NewToolDescription("Lists available voices for a specific TTS provider").
					WhenToUse(
						"BEFORE creating profile, to show voice options to user",
						"When user asks what voices are available",
						"To help user choose a voice",
					).
					Returns("List of available voices with IDs and descriptions").
					Notes(
						"OpenAI voices are fixed: nova, alloy, echo, fable, onyx, shimmer",
						"WebSpeech/SAPI5 voices depend on the user's system",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"provider": JSONSchemaStringEnum(
						NewParamDescription("Provider to list voices for").Build(),
						[]string{"openai", "webspeech", "sapi5"},
					),
				}, []string{"provider"}),
			},
		},

		// ==================== Interaction Profile Tools ====================
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_profile_list",
				Description: NewToolDescription("Lists all interaction profiles (STT configurations)").
					WhenToUse(
						"ALWAYS as FIRST STEP before creating any interaction profile",
						"To check which profile is currently active",
						"To find the ID of an existing profile",
						"To see available interaction configurations",
					).
					Returns("List of profiles with triggers, STT provider, active/default status").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"query": JSONSchemaString(
						NewParamDescription("Optional filter by name or description").Build(),
					),
				}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_profile_get",
				Description: NewToolDescription("Gets complete details of an interaction profile including all triggers").
					WhenToUse(
						"To see all triggers and settings of a profile",
						"Before updating, to see current configuration",
						"When user asks about a specific profile",
					).
					Returns("Full profile details with all triggers and their configurations").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(
						NewParamDescription("Interaction profile ID").Build(),
					),
					"name": JSONSchemaString(
						NewParamDescription("Profile name (alternative to ID)").Build(),
					),
				}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_profile_create",
				Description: NewToolDescription("Creates a new interaction profile with triggers").
					WhenToUse(
						"ONLY when you have ALL required information",
						"After confirming trigger type and settings with user",
						"After checking no similar profile exists",
					).
					WhenNotToUse(
						"If user didn't specify how to activate recording → ASK first",
						"If trigger type is hotkey but no key specified → ASK",
						"If trigger type is wakeword but no word specified → ASK",
						"NEVER assume values user didn't provide",
					).
					Returns("Created profile ID and confirmation").
					Notes(
						"STT providers: webspeech (free), whisper_api (higher quality)",
						"Trigger types: hotkey, button_ptt, button_toggle, wakeword, vad",
						"Hotkey format: 'Ctrl+Shift+Space', 'Alt+R', etc.",
						"A profile can have multiple triggers",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"name": JSONSchemaString(
						NewParamDescription("Unique profile name").
							Examples("Default Interaction", "Dictation Mode", "Quick Commands").Build(),
					),
					"description": JSONSchemaString(
						NewParamDescription("Profile description").Build(),
					),
					"stt_provider": JSONSchemaStringEnum(
						NewParamDescription("Speech recognition provider").
							Default("webspeech").Build(),
						[]string{"webspeech", "whisper_api"},
					),
					"language": JSONSchemaString(
						NewParamDescription("Recognition language").
							Default("pt-BR").
							Examples("pt-BR", "en-US", "es-ES").Build(),
					),
					"feedback_sounds": JSONSchemaBool(
						NewParamDescription("Play sounds on start/stop recording").
							Default("true").Build(),
					),
					"triggers_json": JSONSchemaString(
						NewParamDescription("JSON array of triggers to create with the profile").
							Examples(
								`[{"type":"hotkey","hotkey":"Ctrl+Shift+Space","auto_stop":true}]`,
								`[{"type":"wakeword","wakeword_keyword":"assistant"}]`,
								`[{"type":"button_ptt"},{"type":"hotkey","hotkey":"Alt+R"}]`,
							).Build(),
					),
					"is_default": JSONSchemaBool(
						NewParamDescription("Set as default profile").
							Default("false").Build(),
					),
				}, []string{"name", "stt_provider"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_profile_update",
				Description: NewToolDescription("Updates an existing interaction profile").
					WhenToUse(
						"To change STT provider, language, or other settings",
						"After confirming changes with user",
					).
					Returns("Update confirmation").
					Notes(
						"This updates profile settings, not triggers",
						"Use trigger tools to modify triggers",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id":      JSONSchemaInt(NewParamDescription("Profile ID to update").Build()),
					"name":            JSONSchemaString(NewParamDescription("New name").Build()),
					"description":     JSONSchemaString(NewParamDescription("New description").Build()),
					"stt_provider":    JSONSchemaStringEnum(NewParamDescription("New STT provider").Build(), []string{"webspeech", "whisper_api"}),
					"language":        JSONSchemaString(NewParamDescription("New language").Build()),
					"feedback_sounds": JSONSchemaBool(NewParamDescription("Enable feedback sounds").Build()),
					"is_default":      JSONSchemaBool(NewParamDescription("Set as default").Build()),
				}, []string{"profile_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_profile_delete",
				Description: NewToolDescription("Deletes an interaction profile and all its triggers").
					WhenToUse(
						"When user confirms deletion",
						"To remove unused profiles",
					).
					WhenNotToUse(
						"Without user confirmation - this is irreversible",
						"If it's the active profile - deactivate first",
					).
					Returns("Deletion confirmation").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(NewParamDescription("Profile ID to delete").Build()),
				}, []string{"profile_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_profile_set_default",
				Description: NewToolDescription("Sets an interaction profile as the default").
					WhenToUse(
						"When user wants to change the default interaction profile",
					).
					Returns("Confirmation of new default").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(NewParamDescription("Profile ID to set as default").Build()),
				}, []string{"profile_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_profile_activate",
				Description: NewToolDescription("Activates an interaction profile for use").
					WhenToUse(
						"When user wants to use a specific profile",
						"After creating a new profile user wants to use now",
						"To switch between profiles",
					).
					Returns("Activation confirmation").
					Notes(
						"Only one profile can be active at a time",
						"Activating registers hotkeys automatically",
						"Use profile_id=0 to deactivate all profiles",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(
						NewParamDescription("Profile ID to activate (0 to deactivate all)").Build(),
					),
				}, []string{"profile_id"}),
			},
		},

		// ==================== Trigger Tools ====================
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_trigger_add",
				Description: NewToolDescription("Adds a new trigger to an existing interaction profile").
					WhenToUse(
						"To add a hotkey, wakeword, or other trigger to existing profile",
						"When user wants multiple activation methods",
					).
					Returns("Created trigger ID and confirmation").
					Notes(
						"Types: hotkey, button_ptt, button_toggle, wakeword, vad",
						"For hotkey: provide 'hotkey' (e.g., 'Ctrl+Shift+Space')",
						"For wakeword: provide 'wakeword_keyword' (e.g., 'assistant')",
						"auto_stop=true means recording stops on silence (VAD)",
					).
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"profile_id": JSONSchemaInt(NewParamDescription("Profile ID to add trigger to").Build()),
					"type": JSONSchemaStringEnum(
						NewParamDescription("Trigger type").Build(),
						[]string{"hotkey", "button_ptt", "button_toggle", "wakeword", "vad"},
					),
					"enabled":               JSONSchemaBool(NewParamDescription("Trigger enabled").Default("true").Build()),
					"auto_stop":             JSONSchemaBool(NewParamDescription("Auto-stop on silence (VAD)").Default("false").Build()),
					"hotkey":                JSONSchemaString(NewParamDescription("Keyboard shortcut").Examples("Ctrl+Shift+Space", "Alt+R").Build()),
					"hotkey_global":         JSONSchemaBool(NewParamDescription("Works globally (any app)").Default("true").Build()),
					"hotkey_bring_to_front": JSONSchemaBool(NewParamDescription("Bring window to front").Default("true").Build()),
					"wakeword_keyword":      JSONSchemaString(NewParamDescription("Activation word").Examples("assistant", "computer").Build()),
					"wakeword_sensitivity":  JSONSchemaNumber(NewParamDescription("Sensitivity 0.0-1.0").Default("0.5").Build()),
					"vad_silence_threshold": JSONSchemaNumber(NewParamDescription("Silence threshold").Default("0.01").Build()),
					"vad_silence_duration":  JSONSchemaInt(NewParamDescription("Silence duration (ms)").Default("1500").Build()),
				}, []string{"profile_id", "type"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_trigger_update",
				Description: NewToolDescription("Updates an existing trigger").
					WhenToUse(
						"To change hotkey combination",
						"To adjust VAD sensitivity",
						"To enable/disable a trigger",
					).
					Returns("Update confirmation").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"trigger_id":            JSONSchemaInt(NewParamDescription("Trigger ID to update").Build()),
					"enabled":               JSONSchemaBool(NewParamDescription("Trigger enabled").Build()),
					"auto_stop":             JSONSchemaBool(NewParamDescription("Auto-stop on silence").Build()),
					"hotkey":                JSONSchemaString(NewParamDescription("New hotkey").Build()),
					"hotkey_global":         JSONSchemaBool(NewParamDescription("Global hotkey").Build()),
					"hotkey_bring_to_front": JSONSchemaBool(NewParamDescription("Bring window to front").Build()),
					"wakeword_keyword":      JSONSchemaString(NewParamDescription("New wakeword").Build()),
					"wakeword_sensitivity":  JSONSchemaNumber(NewParamDescription("New sensitivity").Build()),
					"vad_silence_threshold": JSONSchemaNumber(NewParamDescription("New silence threshold").Build()),
					"vad_silence_duration":  JSONSchemaInt(NewParamDescription("New silence duration").Build()),
				}, []string{"trigger_id"}),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "interaction_trigger_delete",
				Description: NewToolDescription("Deletes a trigger from a profile").
					WhenToUse(
						"When user confirms deletion",
						"To remove unused triggers",
					).
					WhenNotToUse(
						"Without user confirmation",
					).
					Returns("Deletion confirmation").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{
					"trigger_id": JSONSchemaInt(NewParamDescription("Trigger ID to delete").Build()),
				}, []string{"trigger_id"}),
			},
		},
	}
}

// ExecuteTool executes a specific tool
func (a *ProfileAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("error parsing arguments: %w", err)
	}

	toolName := toolCall.Function.Name

	switch toolName {
	// Voice Profile
	case "voice_profile_list":
		return a.listVoiceProfiles(args)
	case "voice_profile_get":
		return a.getVoiceProfile(args)
	case "voice_profile_create":
		return a.createVoiceProfile(args)
	case "voice_profile_update":
		return a.updateVoiceProfile(args)
	case "voice_profile_delete":
		return a.deleteVoiceProfile(args)
	case "voice_profile_set_default":
		return a.setDefaultVoiceProfile(args)
	case "voice_profile_apply_to_conversation":
		return a.applyVoiceProfileToConversation(args)
	case "voice_list_available":
		return a.listAvailableVoices(args)

	// Interaction Profile
	case "interaction_profile_list":
		return a.listInteractionProfiles(args)
	case "interaction_profile_get":
		return a.getInteractionProfile(args)
	case "interaction_profile_create":
		return a.createInteractionProfile(args)
	case "interaction_profile_update":
		return a.updateInteractionProfile(args)
	case "interaction_profile_delete":
		return a.deleteInteractionProfile(args)
	case "interaction_profile_set_default":
		return a.setDefaultInteractionProfile(args)
	case "interaction_profile_activate":
		return a.activateInteractionProfile(args)

	// Triggers
	case "interaction_trigger_add":
		return a.addTrigger(args)
	case "interaction_trigger_update":
		return a.updateTrigger(args)
	case "interaction_trigger_delete":
		return a.deleteTrigger(args)

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ==================== Voice Profile Implementation ====================

func (a *ProfileAgent) listVoiceProfiles(args map[string]interface{}) (string, error) {
	query := getStringArg(args, "query", "")

	var profiles []database.VoiceProfile
	var err error

	if query != "" {
		profiles, err = database.SearchVoiceProfiles(query)
	} else {
		profiles, err = database.GetAllVoiceProfiles()
	}

	if err != nil {
		return "", fmt.Errorf("error listing voice profiles: %w", err)
	}

	if len(profiles) == 0 {
		return "No voice profiles found.", nil
	}

	result := fmt.Sprintf("📢 **%d Voice Profile(s)**:\n\n", len(profiles))
	for _, p := range profiles {
		status := ""
		if p.IsDefault {
			status = " ⭐ DEFAULT"
		}
		enabled := ""
		if p.EnabledForAgent {
			enabled += " 🔊Agent"
		}
		if p.EnabledForUser {
			enabled += " 🔊User"
		}
		if enabled == "" {
			enabled = " (disabled)"
		}

		result += fmt.Sprintf("**%s** (ID: %d)%s\n", p.Name, p.ID, status)
		result += fmt.Sprintf("   Provider: %s | Voice: %s | Rate: %.1fx%s\n\n",
			p.Provider, p.VoiceID, p.Rate, enabled)
	}

	return result, nil
}

func (a *ProfileAgent) getVoiceProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	name := getStringArg(args, "name", "")

	var profile *database.VoiceProfile
	var err error

	if profileID > 0 {
		profile, err = database.GetVoiceProfile(uint(profileID))
	} else if name != "" {
		profile, err = database.GetVoiceProfileByName(name)
	} else {
		return "", fmt.Errorf("profile_id or name is required")
	}

	if err != nil {
		return "", fmt.Errorf("error getting voice profile: %w", err)
	}

	status := ""
	if profile.IsDefault {
		status = " ⭐ DEFAULT"
	}

	result := fmt.Sprintf(`📢 **%s** (ID: %d)%s

**Provider**: %s
**Voice**: %s
**Rate**: %.2f
**Pitch**: %.2f
**Volume**: %.2f
**Read Agent Responses**: %v
**Read User Messages**: %v
**Description**: %s`,
		profile.Name, profile.ID, status,
		profile.Provider, profile.VoiceID,
		profile.Rate, profile.Pitch, profile.Volume,
		profile.EnabledForAgent, profile.EnabledForUser,
		profile.Description)

	return result, nil
}

func (a *ProfileAgent) createVoiceProfile(args map[string]interface{}) (string, error) {
	name := getStringArg(args, "name", "")
	provider := getStringArg(args, "provider", "")

	// Semantic validation - return friendly message, not technical error
	if name == "" {
		return `⚠️ **Required information**: What name should this profile have?

Please provide a unique name for the voice profile.`, nil
	}

	if provider == "" {
		return `⚠️ **Required information**: Which TTS provider do you want to use?

**Options:**
- **openai**: High quality, natural voices (requires API key)
- **webspeech**: Free, uses browser voices
- **sapi5**: Offline, uses Windows voices
- **disabled**: No text-to-speech

Please choose a provider.`, nil
	}

	voiceID := getStringArg(args, "voice_id", "")
	if provider != "disabled" && voiceID == "" {
		if provider == "openai" {
			return `⚠️ **Required information**: Which OpenAI voice do you want to use?

**Available voices:**
- **nova**: Female, natural and expressive (recommended)
- **alloy**: Neutral, versatile
- **echo**: Male, clear
- **fable**: Expressive, good for narratives
- **onyx**: Male, deep voice
- **shimmer**: Female, soft

Please choose a voice.`, nil
		}
		return `⚠️ **Required information**: Which voice do you want to use?

Use voice_list_available to see available voices for this provider.`, nil
	}

	opts := database.VoiceProfileOptions{
		Name:            name,
		Description:     getStringArg(args, "description", ""),
		Provider:        provider,
		VoiceID:         voiceID,
		Rate:            getFloatArg(args, "rate", 1.0),
		Pitch:           getFloatArg(args, "pitch", 1.0),
		Volume:          getFloatArg(args, "volume", 1.0),
		EnabledForAgent: getBoolArg(args, "enabled_for_agent", true),
		EnabledForUser:  getBoolArg(args, "enabled_for_user", false),
		IsDefault:       getBoolArg(args, "is_default", false),
	}

	profile, err := database.CreateVoiceProfileFull(opts)
	if err != nil {
		return "", fmt.Errorf("error creating voice profile: %w", err)
	}

	// Emit event for frontend update
	if a.emitEventCallback != nil {
		a.emitEventCallback("voice:profile:created", profile)
	}

	result := fmt.Sprintf(`✅ **Voice Profile Created!**

**ID**: %d
**Name**: %s
**Provider**: %s
**Voice**: %s
**Rate**: %.1fx
**Read Agent Responses**: %v`,
		profile.ID, profile.Name, profile.Provider,
		profile.VoiceID, profile.Rate, profile.EnabledForAgent)

	if profile.IsDefault {
		result += "\n\n⭐ Set as default profile"
	}

	return result, nil
}

func (a *ProfileAgent) updateVoiceProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	if profileID == 0 {
		return "", fmt.Errorf("profile_id is required")
	}

	// Get current profile
	current, err := database.GetVoiceProfile(uint(profileID))
	if err != nil {
		return "", fmt.Errorf("profile not found: %w", err)
	}

	opts := database.VoiceProfileOptions{
		Name:            getStringArgOrDefault(args, "name", current.Name),
		Description:     getStringArgOrDefault(args, "description", current.Description),
		Provider:        getStringArgOrDefault(args, "provider", current.Provider),
		VoiceID:         getStringArgOrDefault(args, "voice_id", current.VoiceID),
		Rate:            getFloatArgOrDefault(args, "rate", current.Rate),
		Pitch:           getFloatArgOrDefault(args, "pitch", current.Pitch),
		Volume:          getFloatArgOrDefault(args, "volume", current.Volume),
		EnabledForAgent: getBoolArgOrDefault(args, "enabled_for_agent", current.EnabledForAgent),
		EnabledForUser:  getBoolArgOrDefault(args, "enabled_for_user", current.EnabledForUser),
		IsDefault:       getBoolArgOrDefault(args, "is_default", current.IsDefault),
	}

	profile, err := database.UpdateVoiceProfileFull(uint(profileID), opts)
	if err != nil {
		return "", fmt.Errorf("error updating voice profile: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("voice:profile:updated", profile)
	}

	return fmt.Sprintf("✅ Voice profile **%s** (ID: %d) updated successfully!", profile.Name, profile.ID), nil
}

func (a *ProfileAgent) deleteVoiceProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	if profileID == 0 {
		return "", fmt.Errorf("profile_id is required")
	}

	// Get profile name before deleting
	profile, _ := database.GetVoiceProfile(uint(profileID))
	name := "Voice Profile"
	if profile != nil {
		name = profile.Name
	}

	if err := database.DeleteVoiceProfile(uint(profileID)); err != nil {
		return "", fmt.Errorf("error deleting voice profile: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("voice:profile:deleted", profileID)
	}

	return fmt.Sprintf("✅ Voice profile **%s** (ID: %d) deleted successfully!", name, profileID), nil
}

func (a *ProfileAgent) setDefaultVoiceProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	if profileID == 0 {
		return "", fmt.Errorf("profile_id is required")
	}

	if err := database.SetDefaultVoiceProfile(uint(profileID)); err != nil {
		return "", fmt.Errorf("error setting default: %w", err)
	}

	profile, _ := database.GetVoiceProfile(uint(profileID))
	name := fmt.Sprintf("ID %d", profileID)
	if profile != nil {
		name = profile.Name
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("voice:profile:default_changed", profileID)
	}

	return fmt.Sprintf("✅ **%s** is now the default voice profile!", name), nil
}

func (a *ProfileAgent) applyVoiceProfileToConversation(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)

	// Check if we have the conversation ID
	if a.ConversationID == 0 {
		return "", fmt.Errorf("no active conversation to apply profile to")
	}

	// Check if callback is configured
	if a.applyVoiceToConversationCallback == nil {
		return "", fmt.Errorf("apply to conversation callback not configured")
	}

	// If profileID is 0, we're removing the custom profile (will use default)
	if profileID == 0 {
		if err := a.applyVoiceToConversationCallback(a.ConversationID, 0); err != nil {
			return "", fmt.Errorf("error removing profile from conversation: %w", err)
		}

		// Emit event
		if a.emitEventCallback != nil {
			a.emitEventCallback("voice:profile:conversation_changed", map[string]interface{}{
				"conversation_id": a.ConversationID,
				"profile_id":      0,
			})
		}

		return "✅ Voice profile removed from this conversation. It will now use the default profile.", nil
	}

	// Verify the profile exists
	profile, err := database.GetVoiceProfile(uint(profileID))
	if err != nil {
		return "", fmt.Errorf("voice profile not found: %w", err)
	}

	// Apply to conversation
	if err := a.applyVoiceToConversationCallback(a.ConversationID, uint(profileID)); err != nil {
		return "", fmt.Errorf("error applying profile to conversation: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("voice:profile:conversation_changed", map[string]interface{}{
			"conversation_id": a.ConversationID,
			"profile_id":      profileID,
			"profile":         profile,
		})
	}

	result := fmt.Sprintf(`✅ **Voice profile applied to this conversation!**

**Profile**: %s (ID: %d)
**Provider**: %s
**Voice**: %s
**Rate**: %.1fx

The assistant will now use this voice configuration for this conversation.`,
		profile.Name, profile.ID, profile.Provider, profile.VoiceID, profile.Rate)

	return result, nil
}

func (a *ProfileAgent) listAvailableVoices(args map[string]interface{}) (string, error) {
	provider := getStringArg(args, "provider", "")
	if provider == "" {
		return "", fmt.Errorf("provider is required")
	}

	switch provider {
	case "openai":
		return `🎤 **OpenAI TTS Voices**:

| Voice | Description |
|-------|-------------|
| **nova** | Female, natural and expressive (most popular) |
| **alloy** | Neutral, versatile for various contexts |
| **echo** | Male, clear and articulate |
| **fable** | Expressive, ideal for storytelling |
| **onyx** | Male, deep and authoritative |
| **shimmer** | Female, soft and gentle |

All voices support multiple languages and have consistent quality.`, nil

	case "webspeech":
		return `🎤 **WebSpeech Voices**:

WebSpeech voices depend on the user's browser and operating system.
Common voices include:
- Google voices (Chrome)
- Microsoft voices (Edge)
- Apple voices (Safari)

The actual available voices vary by system. The user should check their browser's speech settings.`, nil

	case "sapi5":
		return `🎤 **SAPI5 Voices (Windows)**:

SAPI5 voices are installed on the Windows system. Common voices:
- **Microsoft Maria** (pt-BR)
- **Microsoft Daniel** (pt-BR)
- **Microsoft David** (en-US)
- **Microsoft Zira** (en-US)

Additional voices can be installed through Windows Settings > Time & Language > Speech.`, nil

	default:
		return fmt.Sprintf("Unknown provider: %s. Valid options: openai, webspeech, sapi5", provider), nil
	}
}

// ==================== Interaction Profile Implementation ====================

func (a *ProfileAgent) listInteractionProfiles(args map[string]interface{}) (string, error) {
	query := getStringArg(args, "query", "")

	var profiles []database.InteractionProfile
	var err error

	if query != "" {
		profiles, err = database.SearchInteractionProfiles(query)
	} else {
		profiles, err = database.GetAllInteractionProfiles()
	}

	if err != nil {
		return "", fmt.Errorf("error listing interaction profiles: %w", err)
	}

	if len(profiles) == 0 {
		return "No interaction profiles found.", nil
	}

	result := fmt.Sprintf("🎙️ **%d Interaction Profile(s)**:\n\n", len(profiles))
	for _, p := range profiles {
		status := ""
		if p.IsActive {
			status += " ▶️ ACTIVE"
		}
		if p.IsDefault {
			status += " ⭐ DEFAULT"
		}

		triggers := []string{}
		for _, t := range p.Triggers {
			if t.Enabled {
				switch t.Type {
				case "hotkey":
					triggers = append(triggers, fmt.Sprintf("⌨️%s", t.Hotkey))
				case "wakeword":
					triggers = append(triggers, fmt.Sprintf("🗣️\"%s\"", t.WakewordKeyword))
				case "button_ptt":
					triggers = append(triggers, "🔘PTT")
				case "button_toggle":
					triggers = append(triggers, "🔘Toggle")
				case "vad":
					triggers = append(triggers, "🔊VAD")
				}
			}
		}

		triggerStr := "none"
		if len(triggers) > 0 {
			triggerStr = strings.Join(triggers, ", ")
		}

		result += fmt.Sprintf("**%s** (ID: %d)%s\n", p.Name, p.ID, status)
		result += fmt.Sprintf("   STT: %s | Lang: %s | Triggers: %s\n\n",
			p.STTProvider, p.Language, triggerStr)
	}

	return result, nil
}

func (a *ProfileAgent) getInteractionProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	name := getStringArg(args, "name", "")

	var profile *database.InteractionProfile
	var err error

	if profileID > 0 {
		profile, err = database.GetInteractionProfile(uint(profileID))
	} else if name != "" {
		profile, err = database.GetInteractionProfileByName(name)
	} else {
		return "", fmt.Errorf("profile_id or name is required")
	}

	if err != nil {
		return "", fmt.Errorf("error getting interaction profile: %w", err)
	}

	status := ""
	if profile.IsActive {
		status += " ▶️ ACTIVE"
	}
	if profile.IsDefault {
		status += " ⭐ DEFAULT"
	}

	result := fmt.Sprintf(`🎙️ **%s** (ID: %d)%s

**STT Provider**: %s
**Language**: %s
**Feedback Sounds**: %v
**Description**: %s

**Triggers** (%d):`,
		profile.Name, profile.ID, status,
		profile.STTProvider, profile.Language,
		profile.FeedbackSounds, profile.Description,
		len(profile.Triggers))

	if len(profile.Triggers) == 0 {
		result += "\n(No triggers configured)"
	} else {
		for _, t := range profile.Triggers {
			enabled := "✅"
			if !t.Enabled {
				enabled = "❌"
			}
			result += fmt.Sprintf("\n%s **%s** (ID: %d)", enabled, t.Type, t.ID)
			switch t.Type {
			case "hotkey":
				result += fmt.Sprintf(" - %s (global: %v, auto_stop: %v)", t.Hotkey, t.HotkeyGlobal, t.AutoStop)
			case "wakeword":
				result += fmt.Sprintf(" - \"%s\" (sensitivity: %.2f)", t.WakewordKeyword, t.WakewordSensitivity)
			case "vad":
				result += fmt.Sprintf(" - silence: %dms, threshold: %.3f", t.VADSilenceDuration, t.VADSilenceThreshold)
			}
		}
	}

	return result, nil
}

func (a *ProfileAgent) createInteractionProfile(args map[string]interface{}) (string, error) {
	name := getStringArg(args, "name", "")
	sttProvider := getStringArg(args, "stt_provider", "webspeech")

	if name == "" {
		return `⚠️ **Required information**: What name should this profile have?

Please provide a unique name for the interaction profile.`, nil
	}

	profile := &database.InteractionProfile{
		Name:           name,
		Description:    getStringArg(args, "description", ""),
		STTProvider:    sttProvider,
		Language:       getStringArg(args, "language", "pt-BR"),
		FeedbackSounds: getBoolArg(args, "feedback_sounds", true),
		IsDefault:      getBoolArg(args, "is_default", false),
	}

	created, err := database.CreateInteractionProfile(profile)
	if err != nil {
		return "", fmt.Errorf("error creating interaction profile: %w", err)
	}

	// Create triggers if provided
	triggersJSON := getStringArg(args, "triggers_json", "")
	triggersCreated := 0
	if triggersJSON != "" {
		var triggers []map[string]interface{}
		if err := json.Unmarshal([]byte(triggersJSON), &triggers); err == nil {
			for _, t := range triggers {
				trigger := a.buildTriggerFromArgs(created.ID, t)
				if _, err := database.CreateInteractionTrigger(trigger); err == nil {
					triggersCreated++
				}
			}
		}
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:profile:created", created)
	}

	result := fmt.Sprintf(`✅ **Interaction Profile Created!**

**ID**: %d
**Name**: %s
**STT Provider**: %s
**Language**: %s
**Triggers Created**: %d`,
		created.ID, created.Name, created.STTProvider,
		created.Language, triggersCreated)

	if triggersCreated == 0 {
		result += "\n\n💡 Tip: Add triggers with interaction_trigger_add to enable activation methods."
	}

	return result, nil
}

func (a *ProfileAgent) updateInteractionProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	if profileID == 0 {
		return "", fmt.Errorf("profile_id is required")
	}

	current, err := database.GetInteractionProfile(uint(profileID))
	if err != nil {
		return "", fmt.Errorf("profile not found: %w", err)
	}

	updated := &database.InteractionProfile{
		Name:           getStringArgOrDefault(args, "name", current.Name),
		Description:    getStringArgOrDefault(args, "description", current.Description),
		STTProvider:    getStringArgOrDefault(args, "stt_provider", current.STTProvider),
		Language:       getStringArgOrDefault(args, "language", current.Language),
		FeedbackSounds: getBoolArgOrDefault(args, "feedback_sounds", current.FeedbackSounds),
		IsDefault:      getBoolArgOrDefault(args, "is_default", current.IsDefault),
		IsActive:       current.IsActive,
	}

	profile, err := database.UpdateInteractionProfile(uint(profileID), updated)
	if err != nil {
		return "", fmt.Errorf("error updating interaction profile: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:profile:updated", profile)
	}

	return fmt.Sprintf("✅ Interaction profile **%s** (ID: %d) updated successfully!", profile.Name, profile.ID), nil
}

func (a *ProfileAgent) deleteInteractionProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	if profileID == 0 {
		return "", fmt.Errorf("profile_id is required")
	}

	profile, _ := database.GetInteractionProfile(uint(profileID))
	name := "Interaction Profile"
	if profile != nil {
		name = profile.Name
		// Deactivate if it's the active profile
		if profile.IsActive && a.deactivateCallback != nil {
			a.deactivateCallback(uint(profileID))
		}
	}

	if err := database.DeleteInteractionProfile(uint(profileID)); err != nil {
		return "", fmt.Errorf("error deleting interaction profile: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:profile:deleted", profileID)
	}

	return fmt.Sprintf("✅ Interaction profile **%s** (ID: %d) deleted successfully!", name, profileID), nil
}

func (a *ProfileAgent) setDefaultInteractionProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	if profileID == 0 {
		return "", fmt.Errorf("profile_id is required")
	}

	if err := database.SetDefaultInteractionProfile(uint(profileID)); err != nil {
		return "", fmt.Errorf("error setting default: %w", err)
	}

	profile, _ := database.GetInteractionProfile(uint(profileID))
	name := fmt.Sprintf("ID %d", profileID)
	if profile != nil {
		name = profile.Name
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:profile:default_changed", profileID)
	}

	return fmt.Sprintf("✅ **%s** is now the default interaction profile!", name), nil
}

func (a *ProfileAgent) activateInteractionProfile(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)

	// Deactivate current profile first
	current, _ := database.GetActiveInteractionProfile()
	if current != nil && a.deactivateCallback != nil {
		a.deactivateCallback(current.ID)
	}

	// Set active in database
	if err := database.SetActiveInteractionProfile(uint(profileID)); err != nil {
		return "", fmt.Errorf("error activating profile: %w", err)
	}

	if profileID == 0 {
		// Emit event
		if a.emitEventCallback != nil {
			a.emitEventCallback("interaction:profile:deactivated", nil)
		}
		return "✅ All interaction profiles deactivated.", nil
	}

	// Activate new profile (register hotkeys)
	if a.activateCallback != nil {
		if err := a.activateCallback(uint(profileID)); err != nil {
			return "", fmt.Errorf("profile activated but failed to register hotkeys: %w", err)
		}
	}

	profile, _ := database.GetInteractionProfile(uint(profileID))
	name := fmt.Sprintf("ID %d", profileID)
	triggers := ""
	if profile != nil {
		name = profile.Name
		for _, t := range profile.Triggers {
			if t.Enabled && t.Type == "hotkey" {
				triggers += fmt.Sprintf("\n   ⌨️ %s registered", t.Hotkey)
			}
		}
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:profile:activated", profileID)
	}

	result := fmt.Sprintf("✅ Interaction profile **%s** is now active!", name)
	if triggers != "" {
		result += "\n\n**Hotkeys:**" + triggers
	}

	return result, nil
}

// ==================== Trigger Implementation ====================

func (a *ProfileAgent) addTrigger(args map[string]interface{}) (string, error) {
	profileID := getIntArg(args, "profile_id", 0)
	triggerType := getStringArg(args, "type", "")

	if profileID == 0 {
		return "", fmt.Errorf("profile_id is required")
	}
	if triggerType == "" {
		return `⚠️ **Required information**: What type of trigger do you want to add?

**Options:**
- **hotkey**: Keyboard shortcut (e.g., Ctrl+Shift+Space)
- **button_ptt**: Push-to-talk button
- **button_toggle**: Toggle button
- **wakeword**: Activation word (e.g., "assistant")
- **vad**: Voice Activity Detection (automatic)

Please specify the trigger type.`, nil
	}

	// Validate required fields for specific types
	if triggerType == "hotkey" {
		hotkey := getStringArg(args, "hotkey", "")
		if hotkey == "" {
			return `⚠️ **Required information**: Which keyboard shortcut?

**Examples:**
- Ctrl+Shift+Space
- Alt+R
- Ctrl+Alt+V

Please provide the hotkey combination.`, nil
		}
	}

	if triggerType == "wakeword" {
		keyword := getStringArg(args, "wakeword_keyword", "")
		if keyword == "" {
			return `⚠️ **Required information**: Which activation word?

**Examples:**
- "assistant"
- "computer"
- "hey assistant"

Please provide the wakeword.`, nil
		}
	}

	trigger := a.buildTriggerFromArgs(uint(profileID), args)

	created, err := database.CreateInteractionTrigger(trigger)
	if err != nil {
		return "", fmt.Errorf("error creating trigger: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:trigger:created", created)
	}

	result := fmt.Sprintf("✅ **Trigger Created!**\n\n**ID**: %d\n**Type**: %s", created.ID, created.Type)
	switch created.Type {
	case "hotkey":
		result += fmt.Sprintf("\n**Hotkey**: %s\n**Global**: %v\n**Auto-stop**: %v",
			created.Hotkey, created.HotkeyGlobal, created.AutoStop)
	case "wakeword":
		result += fmt.Sprintf("\n**Keyword**: %s\n**Sensitivity**: %.2f",
			created.WakewordKeyword, created.WakewordSensitivity)
	case "vad":
		result += fmt.Sprintf("\n**Silence Duration**: %dms\n**Silence Threshold**: %.3f",
			created.VADSilenceDuration, created.VADSilenceThreshold)
	}

	return result, nil
}

func (a *ProfileAgent) updateTrigger(args map[string]interface{}) (string, error) {
	triggerID := getIntArg(args, "trigger_id", 0)
	if triggerID == 0 {
		return "", fmt.Errorf("trigger_id is required")
	}

	current, err := database.GetInteractionTrigger(uint(triggerID))
	if err != nil {
		return "", fmt.Errorf("trigger not found: %w", err)
	}

	updated := &database.InteractionTrigger{
		Type:                 current.Type,
		Enabled:              getBoolArgOrDefault(args, "enabled", current.Enabled),
		AutoStop:             getBoolArgOrDefault(args, "auto_stop", current.AutoStop),
		Hotkey:               getStringArgOrDefault(args, "hotkey", current.Hotkey),
		HotkeyGlobal:         getBoolArgOrDefault(args, "hotkey_global", current.HotkeyGlobal),
		HotkeyBringToFront:   getBoolArgOrDefault(args, "hotkey_bring_to_front", current.HotkeyBringToFront),
		WakewordKeyword:      getStringArgOrDefault(args, "wakeword_keyword", current.WakewordKeyword),
		WakewordProvider:     current.WakewordProvider,
		WakewordSensitivity:  getFloatArgOrDefault(args, "wakeword_sensitivity", current.WakewordSensitivity),
		VADSilenceThreshold:  getFloatArgOrDefault(args, "vad_silence_threshold", current.VADSilenceThreshold),
		VADSilenceDuration:   getIntArgOrDefault(args, "vad_silence_duration", current.VADSilenceDuration),
		VADActivityThreshold: current.VADActivityThreshold,
		VADActivityDuration:  current.VADActivityDuration,
	}

	trigger, err := database.UpdateInteractionTrigger(uint(triggerID), updated)
	if err != nil {
		return "", fmt.Errorf("error updating trigger: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:trigger:updated", trigger)
	}

	return fmt.Sprintf("✅ Trigger **%s** (ID: %d) updated successfully!", trigger.Type, trigger.ID), nil
}

func (a *ProfileAgent) deleteTrigger(args map[string]interface{}) (string, error) {
	triggerID := getIntArg(args, "trigger_id", 0)
	if triggerID == 0 {
		return "", fmt.Errorf("trigger_id is required")
	}

	trigger, _ := database.GetInteractionTrigger(uint(triggerID))
	triggerType := "Trigger"
	if trigger != nil {
		triggerType = trigger.Type
	}

	if err := database.DeleteInteractionTrigger(uint(triggerID)); err != nil {
		return "", fmt.Errorf("error deleting trigger: %w", err)
	}

	// Emit event
	if a.emitEventCallback != nil {
		a.emitEventCallback("interaction:trigger:deleted", triggerID)
	}

	return fmt.Sprintf("✅ Trigger **%s** (ID: %d) deleted successfully!", triggerType, triggerID), nil
}

// ==================== Helper Functions ====================

func (a *ProfileAgent) buildTriggerFromArgs(profileID uint, args map[string]interface{}) *database.InteractionTrigger {
	return &database.InteractionTrigger{
		ProfileID:            profileID,
		Type:                 getStringArg(args, "type", ""),
		Enabled:              getBoolArg(args, "enabled", true),
		AutoStop:             getBoolArg(args, "auto_stop", false),
		Hotkey:               getStringArg(args, "hotkey", ""),
		HotkeyGlobal:         getBoolArg(args, "hotkey_global", true),
		HotkeyBringToFront:   getBoolArg(args, "hotkey_bring_to_front", true),
		WakewordKeyword:      getStringArg(args, "wakeword_keyword", ""),
		WakewordProvider:     getStringArg(args, "wakeword_provider", "webspeech"),
		WakewordSensitivity:  getFloatArg(args, "wakeword_sensitivity", 0.5),
		VADSilenceThreshold:  getFloatArg(args, "vad_silence_threshold", 0.01),
		VADSilenceDuration:   getIntArg(args, "vad_silence_duration", 1500),
		VADActivityThreshold: getFloatArg(args, "vad_activity_threshold", 0.02),
		VADActivityDuration:  getIntArg(args, "vad_activity_duration", 200),
	}
}

func getFloatArg(args map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := args[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return defaultValue
}

func getStringArgOrDefault(args map[string]interface{}, key, defaultValue string) string {
	if val, ok := args[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func getFloatArgOrDefault(args map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := args[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return defaultValue
}

func getBoolArgOrDefault(args map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := args[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

func getIntArgOrDefault(args map[string]interface{}, key string, defaultValue int) int {
	if val, ok := args[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return defaultValue
}
