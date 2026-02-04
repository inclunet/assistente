export namespace agents {
	
	export class ConversationResult {
	    conversation_id: number;
	    title: string;
	    summary: string;
	    similarity: number;
	    created_at: string;
	    message_count: number;
	
	    static createFrom(source: any = {}) {
	        return new ConversationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.similarity = source["similarity"];
	        this.created_at = source["created_at"];
	        this.message_count = source["message_count"];
	    }
	}
	export class OpenTabResult {
	    tab_id: number;
	    conversation_id: number;
	    title: string;
	    summary?: string;
	    similarity?: number;
	    is_active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OpenTabResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tab_id = source["tab_id"];
	        this.conversation_id = source["conversation_id"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.similarity = source["similarity"];
	        this.is_active = source["is_active"];
	    }
	}
	export class Registry {
	
	
	    static createFrom(source: any = {}) {
	        return new Registry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace config {
	
	export class ChatDefaults {
	    use_tools?: boolean;
	    show_internal_messages?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatDefaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.use_tools = source["use_tools"];
	        this.show_internal_messages = source["show_internal_messages"];
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
	export class EmbeddingsParams {
	    model?: string;
	    dimensions?: number;
	
	    static createFrom(source: any = {}) {
	        return new EmbeddingsParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.dimensions = source["dimensions"];
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
	    brave_api_key?: string;
	    default_model?: string;
	    embeddings_model?: string;
	    image_model?: string;
	    response_timeout?: number;
	    chat_params?: ModelParams;
	    embeddings_params?: EmbeddingsParams;
	    stt_params?: STTParams;
	    chat_defaults?: ChatDefaults;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api_key = source["api_key"];
	        this.api_base_url = source["api_base_url"];
	        this.brave_api_key = source["brave_api_key"];
	        this.default_model = source["default_model"];
	        this.embeddings_model = source["embeddings_model"];
	        this.image_model = source["image_model"];
	        this.response_timeout = source["response_timeout"];
	        this.chat_params = this.convertValues(source["chat_params"], ModelParams);
	        this.embeddings_params = this.convertValues(source["embeddings_params"], EmbeddingsParams);
	        this.stt_params = this.convertValues(source["stt_params"], STTParams);
	        this.chat_defaults = this.convertValues(source["chat_defaults"], ChatDefaults);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class AgentConfig {
	    id: number;
	    name: string;
	    display_name: string;
	    description: string;
	    agent_type: string;
	    model: string;
	    system_prompt: string;
	    config?: string;
	    enabled: boolean;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new AgentConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.agent_type = source["agent_type"];
	        this.model = source["model"];
	        this.system_prompt = source["system_prompt"];
	        this.config = source["config"];
	        this.enabled = source["enabled"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class ChatMessage {
	    id: number;
	    conversationId: number;
	    parentId?: number;
	    role: string;
	    content: string;
	    media?: string;
	    toolCalls?: string;
	    toolResults?: string;
	    toolCallId?: string;
	    agentName?: string;
	    promptTokens?: number;
	    completionTokens?: number;
	    totalTokens?: number;
	    model?: string;
	    createdAt: time.Time;
	
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
	        this.media = source["media"];
	        this.toolCalls = source["toolCalls"];
	        this.toolResults = source["toolResults"];
	        this.toolCallId = source["toolCallId"];
	        this.agentName = source["agentName"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.model = source["model"];
	        this.createdAt = this.convertValues(source["createdAt"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class ChatPreferences {
	    model?: string;
	    temperature?: number;
	    max_tokens?: number;
	    top_p?: number;
	    use_tools?: boolean;
	    show_internal_messages?: boolean;
	    voice?: string;
	    auto_speak?: boolean;
	    voice_volume?: number;
	    voice_rate?: number;
	    voice_profile_id?: number;
	    stt_provider?: string;
	    recording_mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatPreferences(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.max_tokens = source["max_tokens"];
	        this.top_p = source["top_p"];
	        this.use_tools = source["use_tools"];
	        this.show_internal_messages = source["show_internal_messages"];
	        this.voice = source["voice"];
	        this.auto_speak = source["auto_speak"];
	        this.voice_volume = source["voice_volume"];
	        this.voice_rate = source["voice_rate"];
	        this.voice_profile_id = source["voice_profile_id"];
	        this.stt_provider = source["stt_provider"];
	        this.recording_mode = source["recording_mode"];
	    }
	}
	export class ChatProfile {
	    id: number;
	    name: string;
	    description: string;
	    icon: string;
	    is_default: boolean;
	    created_at: time.Time;
	    updated_at: time.Time;
	    model: string;
	    temperature: number;
	    max_tokens: number;
	    top_p: number;
	    response_timeout: number;
	    use_tools: boolean;
	    tools_list: string;
	    system_prompt: string;
	    system_prompt_position: string;
	    include_core_memories: boolean;
	    embeddings_model: string;
	    embeddings_dimensions: number;
	    image_model: string;
	    show_internal_messages: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.is_default = source["is_default"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.max_tokens = source["max_tokens"];
	        this.top_p = source["top_p"];
	        this.response_timeout = source["response_timeout"];
	        this.use_tools = source["use_tools"];
	        this.tools_list = source["tools_list"];
	        this.system_prompt = source["system_prompt"];
	        this.system_prompt_position = source["system_prompt_position"];
	        this.include_core_memories = source["include_core_memories"];
	        this.embeddings_model = source["embeddings_model"];
	        this.embeddings_dimensions = source["embeddings_dimensions"];
	        this.image_model = source["image_model"];
	        this.show_internal_messages = source["show_internal_messages"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	    chat_profile_id?: number;
	    preferences?: string;
	    created_at: time.Time;
	    updated_at: time.Time;
	    messages?: ChatMessage[];
	    message_count: number;
	    summary?: string;
	    embedding_message_count: number;
	    chat_profile?: ChatProfile;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.chat_profile_id = source["chat_profile_id"];
	        this.preferences = source["preferences"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	        this.messages = this.convertValues(source["messages"], ChatMessage);
	        this.message_count = source["message_count"];
	        this.summary = source["summary"];
	        this.embedding_message_count = source["embedding_message_count"];
	        this.chat_profile = this.convertValues(source["chat_profile"], ChatProfile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	    created_at: time.Time;
	    updated_at: time.Time;
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
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	        this.conversation = this.convertValues(source["conversation"], Conversation);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class FAQ {
	    id: number;
	    question: string;
	    answer: string;
	    tags?: string;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new FAQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.question = source["question"];
	        this.answer = source["answer"];
	        this.tags = source["tags"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class HTTPEndpoint {
	    id: number;
	    http_agent_id: number;
	    name: string;
	    description: string;
	    method: string;
	    path_template: string;
	    query_template: string;
	    headers_json: string;
	    body_template: string;
	    parameters: string;
	    response_template: string;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new HTTPEndpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.http_agent_id = source["http_agent_id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.method = source["method"];
	        this.path_template = source["path_template"];
	        this.query_template = source["query_template"];
	        this.headers_json = source["headers_json"];
	        this.body_template = source["body_template"];
	        this.parameters = source["parameters"];
	        this.response_template = source["response_template"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class HTTPAgent {
	    id: number;
	    agent_config_id: number;
	    base_url: string;
	    auth_type: string;
	    auth_config: string;
	    default_headers: string;
	    timeout_seconds: number;
	    retry_count: number;
	    created_at: time.Time;
	    updated_at: time.Time;
	    endpoints?: HTTPEndpoint[];
	
	    static createFrom(source: any = {}) {
	        return new HTTPAgent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.agent_config_id = source["agent_config_id"];
	        this.base_url = source["base_url"];
	        this.auth_type = source["auth_type"];
	        this.auth_config = source["auth_config"];
	        this.default_headers = source["default_headers"];
	        this.timeout_seconds = source["timeout_seconds"];
	        this.retry_count = source["retry_count"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	        this.endpoints = this.convertValues(source["endpoints"], HTTPEndpoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class InteractionTrigger {
	    id: number;
	    profile_id: number;
	    created_at: time.Time;
	    updated_at: time.Time;
	    type: string;
	    enabled: boolean;
	    auto_stop: boolean;
	    hotkey: string;
	    hotkey_global: boolean;
	    hotkey_bring_to_front: boolean;
	    wakeword_keyword: string;
	    wakeword_provider: string;
	    wakeword_sensitivity: number;
	    vad_silence_threshold: number;
	    vad_silence_duration: number;
	    vad_activity_threshold: number;
	    vad_activity_duration: number;
	
	    static createFrom(source: any = {}) {
	        return new InteractionTrigger(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.profile_id = source["profile_id"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
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
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class InteractionProfile {
	    id: number;
	    name: string;
	    description: string;
	    is_default: boolean;
	    is_active: boolean;
	    created_at: time.Time;
	    updated_at: time.Time;
	    stt_provider: string;
	    language: string;
	    feedback_sounds: boolean;
	    triggers?: InteractionTrigger[];
	
	    static createFrom(source: any = {}) {
	        return new InteractionProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.is_default = source["is_default"];
	        this.is_active = source["is_active"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	        this.stt_provider = source["stt_provider"];
	        this.language = source["language"];
	        this.feedback_sounds = source["feedback_sounds"];
	        this.triggers = this.convertValues(source["triggers"], InteractionTrigger);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class MCPAgentDB {
	    id: number;
	    agent_config_id: number;
	    transport_type: string;
	    server_command: string;
	    server_args: string;
	    server_env: string;
	    working_dir: string;
	    server_url: string;
	    auth_type: string;
	    auth_value: string;
	    http_headers: string;
	    execution_mode: string;
	    auto_connect: boolean;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new MCPAgentDB(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.agent_config_id = source["agent_config_id"];
	        this.transport_type = source["transport_type"];
	        this.server_command = source["server_command"];
	        this.server_args = source["server_args"];
	        this.server_env = source["server_env"];
	        this.working_dir = source["working_dir"];
	        this.server_url = source["server_url"];
	        this.auth_type = source["auth_type"];
	        this.auth_value = source["auth_value"];
	        this.http_headers = source["http_headers"];
	        this.execution_mode = source["execution_mode"];
	        this.auto_connect = source["auto_connect"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class Memory {
	    id: number;
	    title: string;
	    content: string;
	    category?: string;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new Memory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.category = source["category"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class ModelCapability {
	    id: number;
	    model_name: string;
	    supports_vision?: boolean;
	    supports_audio?: boolean;
	    supports_video?: boolean;
	    supports_documents?: boolean;
	    supports_tools?: boolean;
	    supports_streaming?: boolean;
	    supports_json?: boolean;
	    last_tested: time.Time;
	    times_used: number;
	    last_error?: string;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new ModelCapability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.model_name = source["model_name"];
	        this.supports_vision = source["supports_vision"];
	        this.supports_audio = source["supports_audio"];
	        this.supports_video = source["supports_video"];
	        this.supports_documents = source["supports_documents"];
	        this.supports_tools = source["supports_tools"];
	        this.supports_streaming = source["supports_streaming"];
	        this.supports_json = source["supports_json"];
	        this.last_tested = this.convertValues(source["last_tested"], time.Time);
	        this.times_used = source["times_used"];
	        this.last_error = source["last_error"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class OAuthConnection {
	    id: number;
	    provider_id: string;
	    provider_name: string;
	    user_email: string;
	    user_name: string;
	    user_id: string;
	    token_type: string;
	    scopes: string;
	    expires_at: time.Time;
	    is_active: boolean;
	    last_used_at: time.Time;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new OAuthConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider_id = source["provider_id"];
	        this.provider_name = source["provider_name"];
	        this.user_email = source["user_email"];
	        this.user_name = source["user_name"];
	        this.user_id = source["user_id"];
	        this.token_type = source["token_type"];
	        this.scopes = source["scopes"];
	        this.expires_at = this.convertValues(source["expires_at"], time.Time);
	        this.is_active = source["is_active"];
	        this.last_used_at = this.convertValues(source["last_used_at"], time.Time);
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class VoiceProfile {
	    id: number;
	    name: string;
	    description: string;
	    provider: string;
	    voice_id: string;
	    rate: number;
	    pitch: number;
	    volume: number;
	    enabled_for_agent: boolean;
	    enabled_for_user: boolean;
	    is_default: boolean;
	    created_at: time.Time;
	    updated_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new VoiceProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.provider = source["provider"];
	        this.voice_id = source["voice_id"];
	        this.rate = source["rate"];
	        this.pitch = source["pitch"];
	        this.volume = source["volume"];
	        this.enabled_for_agent = source["enabled_for_agent"];
	        this.enabled_for_user = source["enabled_for_user"];
	        this.is_default = source["is_default"];
	        this.created_at = this.convertValues(source["created_at"], time.Time);
	        this.updated_at = this.convertValues(source["updated_at"], time.Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class ChatDefaults {
	    use_tools?: boolean;
	    show_internal_messages?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatDefaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.use_tools = source["use_tools"];
	        this.show_internal_messages = source["show_internal_messages"];
	    }
	}
	export class ChatParams {
	    model: string;
	    maxTokens: number;
	    temperature: number;
	    topP?: number;
	    useTools: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.maxTokens = source["maxTokens"];
	        this.temperature = source["temperature"];
	        this.topP = source["topP"];
	        this.useTools = source["useTools"];
	    }
	}
	export class EmbeddingsParams {
	    model: string;
	    dimensions?: number;
	
	    static createFrom(source: any = {}) {
	        return new EmbeddingsParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.dimensions = source["dimensions"];
	    }
	}
	export class FunctionCall {
	    name: string;
	    arguments: string;
	
	    static createFrom(source: any = {}) {
	        return new FunctionCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.arguments = source["arguments"];
	    }
	}
	export class ToolCall {
	    id: string;
	    type: string;
	    function: FunctionCall;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.function = this.convertValues(source["function"], FunctionCall);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class Message {
	    role: string;
	    content?: any;
	    tool_calls?: ToolCall[];
	    tool_call_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
	        this.tool_call_id = source["tool_call_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	    brave_api_key?: string;
	    response_timeout?: number;
	    chat_params: ModelParams;
	    embeddings_params: EmbeddingsParams;
	    image_model?: string;
	    stt_params?: STTParams;
	    chat_defaults?: ChatDefaults;
	
	    static createFrom(source: any = {}) {
	        return new SettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api_key = source["api_key"];
	        this.api_base_url = source["api_base_url"];
	        this.brave_api_key = source["brave_api_key"];
	        this.response_timeout = source["response_timeout"];
	        this.chat_params = this.convertValues(source["chat_params"], ModelParams);
	        this.embeddings_params = this.convertValues(source["embeddings_params"], EmbeddingsParams);
	        this.image_model = source["image_model"];
	        this.stt_params = this.convertValues(source["stt_params"], STTParams);
	        this.chat_defaults = this.convertValues(source["chat_defaults"], ChatDefaults);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class ToolFunction {
	    name: string;
	    description: string;
	    parameters?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new ToolFunction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.parameters = source["parameters"];
	    }
	}
	export class Tool {
	    type: string;
	    function: ToolFunction;
	
	    static createFrom(source: any = {}) {
	        return new Tool(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.function = this.convertValues(source["function"], ToolFunction);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class AgentInfo {
	    name: string;
	    display_name: string;
	    description: string;
	    agent_type: string;
	    model: string;
	    system_prompt: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.agent_type = source["agent_type"];
	        this.model = source["model"];
	        this.system_prompt = source["system_prompt"];
	        this.enabled = source["enabled"];
	    }
	}
	export class ConversationEmbeddingStatus {
	    total: number;
	    with_embeddings: number;
	    without_embedding: number;
	
	    static createFrom(source: any = {}) {
	        return new ConversationEmbeddingStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.with_embeddings = source["with_embeddings"];
	        this.without_embedding = source["without_embedding"];
	    }
	}
	export class EnrichedMessage {
	    id: string;
	    conversationId: number;
	    parentId?: string;
	    role: string;
	    content: string;
	    media?: string;
	    toolCalls?: string;
	    toolResults?: string;
	    toolCallId?: string;
	    agentName?: string;
	    promptTokens?: number;
	    completionTokens?: number;
	    totalTokens?: number;
	    model?: string;
	    createdAt: time.Time;
	    timestamp: number;
	    isStreaming: boolean;
	    toolName?: string;
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
	        this.media = source["media"];
	        this.toolCalls = source["toolCalls"];
	        this.toolResults = source["toolResults"];
	        this.toolCallId = source["toolCallId"];
	        this.agentName = source["agentName"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.model = source["model"];
	        this.createdAt = this.convertValues(source["createdAt"], time.Time);
	        this.timestamp = source["timestamp"];
	        this.isStreaming = source["isStreaming"];
	        this.toolName = source["toolName"];
	        this.internal = source["internal"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	    preferences?: database.ChatPreferences;
	    threads: MessageNode[];
	
	    static createFrom(source: any = {}) {
	        return new ConversationWithThreads(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.preferences = this.convertValues(source["preferences"], database.ChatPreferences);
	        this.threads = this.convertValues(source["threads"], MessageNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class FAQEmbeddingStatus {
	    total_faqs: number;
	    with_embedding: number;
	    without_embedding: number;
	
	    static createFrom(source: any = {}) {
	        return new FAQEmbeddingStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_faqs = source["total_faqs"];
	        this.with_embedding = source["with_embedding"];
	        this.without_embedding = source["without_embedding"];
	    }
	}
	export class FileAgentAuthorizedPathInfo {
	    id: number;
	    path: string;
	    allow_delete: boolean;
	    allow_write: boolean;
	    recursive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileAgentAuthorizedPathInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.allow_delete = source["allow_delete"];
	        this.allow_write = source["allow_write"];
	        this.recursive = source["recursive"];
	    }
	}
	export class HTTPEndpointInfo {
	    id: number;
	    name: string;
	    description: string;
	    method: string;
	    path_template: string;
	    query_template: string;
	    headers_json: string;
	    body_template: string;
	    parameters: string;
	    response_template: string;
	
	    static createFrom(source: any = {}) {
	        return new HTTPEndpointInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.method = source["method"];
	        this.path_template = source["path_template"];
	        this.query_template = source["query_template"];
	        this.headers_json = source["headers_json"];
	        this.body_template = source["body_template"];
	        this.parameters = source["parameters"];
	        this.response_template = source["response_template"];
	    }
	}
	export class HTTPAgentFullConfig {
	    id: number;
	    name: string;
	    display_name: string;
	    description: string;
	    model: string;
	    system_prompt: string;
	    enabled: boolean;
	    http_agent_id: number;
	    base_url: string;
	    auth_type: string;
	    auth_config: string;
	    default_headers: string;
	    timeout_seconds: number;
	    retry_count: number;
	    endpoints: HTTPEndpointInfo[];
	
	    static createFrom(source: any = {}) {
	        return new HTTPAgentFullConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.model = source["model"];
	        this.system_prompt = source["system_prompt"];
	        this.enabled = source["enabled"];
	        this.http_agent_id = source["http_agent_id"];
	        this.base_url = source["base_url"];
	        this.auth_type = source["auth_type"];
	        this.auth_config = source["auth_config"];
	        this.default_headers = source["default_headers"];
	        this.timeout_seconds = source["timeout_seconds"];
	        this.retry_count = source["retry_count"];
	        this.endpoints = this.convertValues(source["endpoints"], HTTPEndpointInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	
	export class HotkeyInfo {
	    id: number;
	    modifiers: string;
	    key: string;
	    description: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HotkeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.modifiers = source["modifiers"];
	        this.key = source["key"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
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
	export class ImportedEndpoint {
	    name: string;
	    description: string;
	    method: string;
	    path_template: string;
	    query_template: string;
	    headers_json: string;
	    body_template: string;
	    parameters: Record<string, any>;
	    response_template: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportedEndpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.method = source["method"];
	        this.path_template = source["path_template"];
	        this.query_template = source["query_template"];
	        this.headers_json = source["headers_json"];
	        this.body_template = source["body_template"];
	        this.parameters = source["parameters"];
	        this.response_template = source["response_template"];
	    }
	}
	export class MCPPromptArgInfo {
	    name: string;
	    description: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MCPPromptArgInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.required = source["required"];
	    }
	}
	export class MCPPromptInfo {
	    name: string;
	    description: string;
	    arguments: MCPPromptArgInfo[];
	
	    static createFrom(source: any = {}) {
	        return new MCPPromptInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.arguments = this.convertValues(source["arguments"], MCPPromptArgInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class MCPPromptMessageInfo {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPPromptMessageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class MCPPromptResultInfo {
	    description: string;
	    messages: MCPPromptMessageInfo[];
	
	    static createFrom(source: any = {}) {
	        return new MCPPromptResultInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.messages = this.convertValues(source["messages"], MCPPromptMessageInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class MCPResourceContentInfo {
	    uri: string;
	    mime_type: string;
	    text: string;
	    is_blob: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MCPResourceContentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.mime_type = source["mime_type"];
	        this.text = source["text"];
	        this.is_blob = source["is_blob"];
	    }
	}
	export class MCPResourceInfo {
	    uri: string;
	    name: string;
	    description: string;
	    mime_type: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPResourceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.mime_type = source["mime_type"];
	    }
	}
	export class MCPResourceTemplateInfo {
	    uri_template: string;
	    name: string;
	    description: string;
	    mime_type: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPResourceTemplateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri_template = source["uri_template"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.mime_type = source["mime_type"];
	    }
	}
	export class MCPSamplingMessageInfo {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPSamplingMessageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class MCPSamplingRequestInfo {
	    messages: MCPSamplingMessageInfo[];
	    system_prompt: string;
	    max_tokens: number;
	    temperature?: number;
	
	    static createFrom(source: any = {}) {
	        return new MCPSamplingRequestInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], MCPSamplingMessageInfo);
	        this.system_prompt = source["system_prompt"];
	        this.max_tokens = source["max_tokens"];
	        this.temperature = source["temperature"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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
	export class MCPSamplingResultInfo {
	    role: string;
	    content: string;
	    model: string;
	    stop_reason: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPSamplingResultInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.model = source["model"];
	        this.stop_reason = source["stop_reason"];
	    }
	}
	export class MemoryEmbeddingStatus {
	    total: number;
	    with_embeddings: number;
	    without_embedding: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryEmbeddingStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.with_embeddings = source["with_embeddings"];
	        this.without_embedding = source["without_embedding"];
	    }
	}
	
	export class OAuthConnectionInfo {
	    id: number;
	    provider_id: string;
	    provider_name: string;
	    provider_icon: string;
	    user_email: string;
	    user_name: string;
	    scopes: string;
	    is_expired: boolean;
	    expires_at: string;
	    last_used_at: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new OAuthConnectionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider_id = source["provider_id"];
	        this.provider_name = source["provider_name"];
	        this.provider_icon = source["provider_icon"];
	        this.user_email = source["user_email"];
	        this.user_name = source["user_name"];
	        this.scopes = source["scopes"];
	        this.is_expired = source["is_expired"];
	        this.expires_at = source["expires_at"];
	        this.last_used_at = source["last_used_at"];
	        this.created_at = source["created_at"];
	    }
	}
	export class OAuthProviderInfo {
	    id: string;
	    name: string;
	    icon: string;
	    is_configured: boolean;
	    default_scopes: string[];
	
	    static createFrom(source: any = {}) {
	        return new OAuthProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.is_configured = source["is_configured"];
	        this.default_scopes = source["default_scopes"];
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
	export class OpenAPIImportResult {
	    display_name: string;
	    description: string;
	    base_url: string;
	    auth_type: string;
	    auth_config: Record<string, string>;
	    default_headers: Record<string, string>;
	    endpoints: ImportedEndpoint[];
	
	    static createFrom(source: any = {}) {
	        return new OpenAPIImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.base_url = source["base_url"];
	        this.auth_type = source["auth_type"];
	        this.auth_config = source["auth_config"];
	        this.default_headers = source["default_headers"];
	        this.endpoints = this.convertValues(source["endpoints"], ImportedEndpoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
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

export namespace time {
	
	export class Time {
	
	
	    static createFrom(source: any = {}) {
	        return new Time(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

