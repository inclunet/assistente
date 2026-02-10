export namespace config {
	
	export class STTParams {
	    provider?: string;
	    recording_mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new STTParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.recording_mode = source["recording_mode"];
	    }
	}
	export class ModelParams {
	    model?: string;
	    temperature?: number;
	    max_tokens?: number;
	    top_p?: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.max_tokens = source["max_tokens"];
	        this.top_p = source["top_p"];
	    }
	}
	export class Config {
	    api_key: string;
	    api_base_url: string;
	    default_model?: string;
	    response_timeout?: number;
	    active_profile?: string;
	    chat_params?: ModelParams;
	    stt_params?: STTParams;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api_key = source["api_key"];
	        this.api_base_url = source["api_base_url"];
	        this.default_model = source["default_model"];
	        this.response_timeout = source["response_timeout"];
	        this.active_profile = source["active_profile"];
	        this.chat_params = this.convertValues(source["chat_params"], ModelParams);
	        this.stt_params = this.convertValues(source["stt_params"], STTParams);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace database {
	
	export class ChatMessage {
	    id: number;
	    conversationId: number;
	    parentId?: number;
	    role: string;
	    content: string;
	    reasoning?: string;
	    media?: string;
	    promptTokens?: number;
	    completionTokens?: number;
	    totalTokens?: number;
	    model?: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversationId = source["conversationId"];
	        this.parentId = source["parentId"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning = source["reasoning"];
	        this.media = source["media"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.model = source["model"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Conversation {
	    id: number;
	    title: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    messages?: ChatMessage[];
	    message_count: number;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.messages = this.convertValues(source["messages"], ChatMessage);
	        this.message_count = source["message_count"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChatTab {
	    id: number;
	    conversation_id?: number;
	    title: string;
	    icon: string;
	    position: number;
	    is_active: boolean;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    conversation?: Conversation;
	
	    static createFrom(source: any = {}) {
	        return new ChatTab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.title = source["title"];
	        this.icon = source["icon"];
	        this.position = source["position"];
	        this.is_active = source["is_active"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.conversation = this.convertValues(source["conversation"], Conversation);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace llm {
	
	export class ChatParams {
	    model: string;
	    maxTokens: number;
	    temperature: number;
	    topP?: number;
	    enableThinking?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.maxTokens = source["maxTokens"];
	        this.temperature = source["temperature"];
	        this.topP = source["topP"];
	        this.enableThinking = source["enableThinking"];
	    }
	}
	export class Message {
	    role: string;
	    content?: any;
	    thinking?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.thinking = source["thinking"];
	    }
	}
	export class ModelParams {
	    model: string;
	    max_tokens: number;
	    temperature: number;
	    top_p?: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.max_tokens = source["max_tokens"];
	        this.temperature = source["temperature"];
	        this.top_p = source["top_p"];
	    }
	}
	export class STTParams {
	    provider?: string;
	    recording_mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new STTParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.recording_mode = source["recording_mode"];
	    }
	}
	export class SettingsInput {
	    api_key: string;
	    api_base_url: string;
	    response_timeout?: number;
	    chat_params: ModelParams;
	    stt_params?: STTParams;
	
	    static createFrom(source: any = {}) {
	        return new SettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api_key = source["api_key"];
	        this.api_base_url = source["api_base_url"];
	        this.response_timeout = source["response_timeout"];
	        this.chat_params = this.convertValues(source["chat_params"], ModelParams);
	        this.stt_params = this.convertValues(source["stt_params"], STTParams);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class EnrichedMessage {
	    id: string;
	    conversationId: number;
	    parentId?: string;
	    role: string;
	    content: string;
	    reasoning?: string;
	    media?: string;
	    promptTokens?: number;
	    completionTokens?: number;
	    totalTokens?: number;
	    model?: string;
	    // Go type: time
	    createdAt: any;
	    timestamp: number;
	    isStreaming: boolean;
	    internal: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EnrichedMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversationId = source["conversationId"];
	        this.parentId = source["parentId"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning = source["reasoning"];
	        this.media = source["media"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.model = source["model"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.timestamp = source["timestamp"];
	        this.isStreaming = source["isStreaming"];
	        this.internal = source["internal"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MessageNode {
	    message: EnrichedMessage;
	    children?: MessageNode[];
	    level: number;
	    childCount: number;
	
	    static createFrom(source: any = {}) {
	        return new MessageNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = this.convertValues(source["message"], EnrichedMessage);
	        this.children = this.convertValues(source["children"], MessageNode);
	        this.level = source["level"];
	        this.childCount = source["childCount"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConversationWithThreads {
	    id: number;
	    title: string;
	    threads: MessageNode[];
	
	    static createFrom(source: any = {}) {
	        return new ConversationWithThreads(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.threads = this.convertValues(source["threads"], MessageNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ImportResult {
	    success: boolean;
	    imported: number;
	    skipped: number;
	    errors?: string[];
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.imported = source["imported"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	        this.message = source["message"];
	    }
	}
	
	export class OpenAITTSVoiceInfo {
	    id: string;
	    name: string;
	    description: string;
	    gender: string;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenAITTSVoiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.gender = source["gender"];
	        this.provider = source["provider"];
	    }
	}
	export class SAPI5VoiceInfo {
	    id: string;
	    name: string;
	    language: string;
	    gender: string;
	    age: string;
	    vendor: string;
	    description: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new SAPI5VoiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.language = source["language"];
	        this.gender = source["gender"];
	        this.age = source["age"];
	        this.vendor = source["vendor"];
	        this.description = source["description"];
	        this.source = source["source"];
	    }
	}
	export class SynthesisResultInfo {
	    audioBase64: string;
	    format: string;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new SynthesisResultInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audioBase64 = source["audioBase64"];
	        this.format = source["format"];
	        this.provider = source["provider"];
	    }
	}
	export class TabsResponse {
	    tabs: database.ChatTab[];
	    active_tab_id: number;
	
	    static createFrom(source: any = {}) {
	        return new TabsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tabs = this.convertValues(source["tabs"], database.ChatTab);
	        this.active_tab_id = source["active_tab_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TranscriptionResultInfo {
	    text: string;
	    language?: string;
	    duration?: number;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new TranscriptionResultInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.language = source["language"];
	        this.duration = source["duration"];
	        this.provider = source["provider"];
	    }
	}

}

export namespace profiles {
	
	export class ChatConfig {
	    model?: string;
	    temperature: number;
	    max_tokens: number;
	    top_p: number;
	    response_timeout: number;
	    enable_thinking: boolean;
	    system_prompt?: string;
	    system_prompt_position?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.max_tokens = source["max_tokens"];
	        this.top_p = source["top_p"];
	        this.response_timeout = source["response_timeout"];
	        this.enable_thinking = source["enable_thinking"];
	        this.system_prompt = source["system_prompt"];
	        this.system_prompt_position = source["system_prompt_position"];
	    }
	}
	export class TriggerConfig {
	    type: string;
	    enabled: boolean;
	    auto_stop?: boolean;
	    hotkey?: string;
	    hotkey_global?: boolean;
	    hotkey_bring_to_front?: boolean;
	    wakeword_keyword?: string;
	    wakeword_provider?: string;
	    wakeword_sensitivity?: number;
	    vad_silence_threshold?: number;
	    vad_silence_duration?: number;
	    vad_activity_threshold?: number;
	    vad_activity_duration?: number;
	
	    static createFrom(source: any = {}) {
	        return new TriggerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.enabled = source["enabled"];
	        this.auto_stop = source["auto_stop"];
	        this.hotkey = source["hotkey"];
	        this.hotkey_global = source["hotkey_global"];
	        this.hotkey_bring_to_front = source["hotkey_bring_to_front"];
	        this.wakeword_keyword = source["wakeword_keyword"];
	        this.wakeword_provider = source["wakeword_provider"];
	        this.wakeword_sensitivity = source["wakeword_sensitivity"];
	        this.vad_silence_threshold = source["vad_silence_threshold"];
	        this.vad_silence_duration = source["vad_silence_duration"];
	        this.vad_activity_threshold = source["vad_activity_threshold"];
	        this.vad_activity_duration = source["vad_activity_duration"];
	    }
	}
	export class InteractionConfig {
	    stt_provider: string;
	    language: string;
	    feedback_sounds: boolean;
	    triggers?: TriggerConfig[];
	
	    static createFrom(source: any = {}) {
	        return new InteractionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stt_provider = source["stt_provider"];
	        this.language = source["language"];
	        this.feedback_sounds = source["feedback_sounds"];
	        this.triggers = this.convertValues(source["triggers"], TriggerConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VoiceConfig {
	    provider: string;
	    voice_id?: string;
	    rate: number;
	    pitch: number;
	    volume: number;
	    enabled_for_agent: boolean;
	    enabled_for_user: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VoiceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.voice_id = source["voice_id"];
	        this.rate = source["rate"];
	        this.pitch = source["pitch"];
	        this.volume = source["volume"];
	        this.enabled_for_agent = source["enabled_for_agent"];
	        this.enabled_for_user = source["enabled_for_user"];
	    }
	}
	export class Profile {
	    name: string;
	    description?: string;
	    icon?: string;
	    chat: ChatConfig;
	    voice: VoiceConfig;
	    interaction: InteractionConfig;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.chat = this.convertValues(source["chat"], ChatConfig);
	        this.voice = this.convertValues(source["voice"], VoiceConfig);
	        this.interaction = this.convertValues(source["interaction"], InteractionConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (!classs) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProfileInfo {
	    name: string;
	    slug: string;
	    description: string;
	    icon: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.slug = source["slug"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.source = source["source"];
	    }
	}
	

}

