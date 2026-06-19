export namespace allowlist {
	
	export class CommandRule {
	    program: string;
	    subcommands?: string[];
	    args?: string[];
	    decision: string;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new CommandRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.program = source["program"];
	        this.subcommands = source["subcommands"];
	        this.args = source["args"];
	        this.decision = source["decision"];
	        this.description = source["description"];
	    }
	}
	export class Allowlist {
	    name: string;
	    description?: string;
	    auto_approve: string[];
	    always_deny: string[];
	    command_rules?: CommandRule[];
	    default_action: string;
	
	    static createFrom(source: any = {}) {
	        return new Allowlist(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.auto_approve = source["auto_approve"];
	        this.always_deny = source["always_deny"];
	        this.command_rules = this.convertValues(source["command_rules"], CommandRule);
	        this.default_action = source["default_action"];
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
	export class AllowlistInfo {
	    slug: string;
	    name: string;
	    description?: string;
	    ruleCount: number;
	
	    static createFrom(source: any = {}) {
	        return new AllowlistInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.slug = source["slug"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.ruleCount = source["ruleCount"];
	    }
	}

}

export namespace app {
	
	export class AuthStatus {
	    vaultConfigured: boolean;
	    vaultUnlocked: boolean;
	    hasUsers: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AuthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vaultConfigured = source["vaultConfigured"];
	        this.vaultUnlocked = source["vaultUnlocked"];
	        this.hasUsers = source["hasUsers"];
	    }
	}
	export class AuthUser {
	    userId: string;
	    sessionId: string;
	    role: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userId = source["userId"];
	        this.sessionId = source["sessionId"];
	        this.role = source["role"];
	    }
	}
	export class ChatSpeakRequest {
	    conversationId: string;
	    messageId?: string;
	    profileSlug?: string;
	    role: string;
	    text: string;
	    origin: string;
	    interrupt?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatSpeakRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversationId = source["conversationId"];
	        this.messageId = source["messageId"];
	        this.profileSlug = source["profileSlug"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.origin = source["origin"];
	        this.interrupt = source["interrupt"];
	    }
	}
	export class ConversationSummaryInfo {
	    summary: string;
	    summary_up_to_message_id: string;
	    summarizing_in_progress: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConversationSummaryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.summary_up_to_message_id = source["summary_up_to_message_id"];
	        this.summarizing_in_progress = source["summarizing_in_progress"];
	    }
	}
	export class CreateAdminRequest {
	    username: string;
	    displayName: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateAdminRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.displayName = source["displayName"];
	        this.password = source["password"];
	    }
	}
	export class CustomActionView {
	    id: string;
	    label: string;
	    icon?: string;
	    danger?: boolean;
	    confirm?: string;
	    hasEvent: boolean;
	    hasLink: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CustomActionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.danger = source["danger"];
	        this.confirm = source["confirm"];
	        this.hasEvent = source["hasEvent"];
	        this.hasLink = source["hasLink"];
	    }
	}
	export class EditorFileInfo {
	    path: string;
	    exists: boolean;
	    isDir: boolean;
	    size: number;
	    modTimeMs: number;
	
	    static createFrom(source: any = {}) {
	        return new EditorFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.exists = source["exists"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modTimeMs = source["modTimeMs"];
	    }
	}
	export class EditorMergeSession {
	    originalPath: string;
	    mineDraftId: string;
	    diskDraftId: string;
	    conflictDraftId: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new EditorMergeSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.originalPath = source["originalPath"];
	        this.mineDraftId = source["mineDraftId"];
	        this.diskDraftId = source["diskDraftId"];
	        this.conflictDraftId = source["conflictDraftId"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class EditorOpenResult {
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new EditorOpenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}
	export class EditorState {
	    fileModeByPath?: Record<string, string>;
	    mergeSessionsByTabId?: Record<string, EditorMergeSession>;
	
	    static createFrom(source: any = {}) {
	        return new EditorState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fileModeByPath = source["fileModeByPath"];
	        this.mergeSessionsByTabId = this.convertValues(source["mergeSessionsByTabId"], EditorMergeSession, true);
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
	export class LoginRequest {
	    username: string;
	    password: string;
	    clientLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.password = source["password"];
	        this.clientLabel = source["clientLabel"];
	    }
	}
	export class LogoutRequest {
	    refreshToken: string;
	
	    static createFrom(source: any = {}) {
	        return new LogoutRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.refreshToken = source["refreshToken"];
	    }
	}
	export class RefreshRequest {
	    refreshToken: string;
	
	    static createFrom(source: any = {}) {
	        return new RefreshRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.refreshToken = source["refreshToken"];
	    }
	}
	export class RuntimeSubsystemFailure {
	    subsystem: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeSubsystemFailure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subsystem = source["subsystem"];
	    }
	}
	export class RuntimePartialInitPayload {
	    subsystems: RuntimeSubsystemFailure[];
	
	    static createFrom(source: any = {}) {
	        return new RuntimePartialInitPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subsystems = this.convertValues(source["subsystems"], RuntimeSubsystemFailure);
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
	
	export class RuntimeToolCatalogEntry {
	    id: string;
	    userId?: string;
	    mcpServerId?: string;
	    name: string;
	    displayName: string;
	    description?: string;
	    origin: string;
	    category?: string;
	    class?: string;
	    package?: string;
	    risk?: string;
	    schema?: number[];
	    schemaHash?: string;
	    schemaBytes?: number;
	    tags?: string[];
	    availabilityStatus: string;
	    availabilityReason?: string;
	    // Go type: time
	    lastSeenAt?: any;
	    // Go type: time
	    lastAvailableAt?: any;
	    // Go type: time
	    lastUnavailableAt?: any;
	    // Go type: time
	    lastTestedAt?: any;
	    lastTestStatus?: string;
	    lastTestError?: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeToolCatalogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.userId = source["userId"];
	        this.mcpServerId = source["mcpServerId"];
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.description = source["description"];
	        this.origin = source["origin"];
	        this.category = source["category"];
	        this.class = source["class"];
	        this.package = source["package"];
	        this.risk = source["risk"];
	        this.schema = source["schema"];
	        this.schemaHash = source["schemaHash"];
	        this.schemaBytes = source["schemaBytes"];
	        this.tags = source["tags"];
	        this.availabilityStatus = source["availabilityStatus"];
	        this.availabilityReason = source["availabilityReason"];
	        this.lastSeenAt = this.convertValues(source["lastSeenAt"], null);
	        this.lastAvailableAt = this.convertValues(source["lastAvailableAt"], null);
	        this.lastUnavailableAt = this.convertValues(source["lastUnavailableAt"], null);
	        this.lastTestedAt = this.convertValues(source["lastTestedAt"], null);
	        this.lastTestStatus = source["lastTestStatus"];
	        this.lastTestError = source["lastTestError"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class RuntimeToolCatalogFilter {
	    origin?: string;
	    mcpServerId?: string;
	    category?: string;
	    class?: string;
	    package?: string;
	    risk?: string;
	    availabilityStatus?: string;
	    includeUnavailable?: boolean;
	    limit?: number;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeToolCatalogFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origin = source["origin"];
	        this.mcpServerId = source["mcpServerId"];
	        this.category = source["category"];
	        this.class = source["class"];
	        this.package = source["package"];
	        this.risk = source["risk"];
	        this.availabilityStatus = source["availabilityStatus"];
	        this.includeUnavailable = source["includeUnavailable"];
	        this.limit = source["limit"];
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

}

export namespace channels {
	
	export class ChannelConfig {
	    enabled: boolean;
	    bot_token?: string;
	    bot_token_ref?: string;
	    app_token?: string;
	    app_token_ref?: string;
	    api_token?: string;
	    api_token_ref?: string;
	    account?: string;
	    api_url?: string;
	    profile?: string;
	    max_history?: number;
	    max_contacts?: number;
	    owner_user_id?: string;
	    conversations?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ChannelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.bot_token = source["bot_token"];
	        this.bot_token_ref = source["bot_token_ref"];
	        this.app_token = source["app_token"];
	        this.app_token_ref = source["app_token_ref"];
	        this.api_token = source["api_token"];
	        this.api_token_ref = source["api_token_ref"];
	        this.account = source["account"];
	        this.api_url = source["api_url"];
	        this.profile = source["profile"];
	        this.max_history = source["max_history"];
	        this.max_contacts = source["max_contacts"];
	        this.owner_user_id = source["owner_user_id"];
	        this.conversations = source["conversations"];
	    }
	}
	export class ChannelTemplateField {
	    key: string;
	    label: string;
	    type: string;
	    required: boolean;
	    placeholder?: string;
	    description?: string;
	    default_value?: any;
	
	    static createFrom(source: any = {}) {
	        return new ChannelTemplateField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.placeholder = source["placeholder"];
	        this.description = source["description"];
	        this.default_value = source["default_value"];
	    }
	}
	export class ChannelTemplate {
	    type: string;
	    display_name: string;
	    description: string;
	    icon: string;
	    fields: ChannelTemplateField[];
	    doc_url: string;
	    supported: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChannelTemplate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.fields = this.convertValues(source["fields"], ChannelTemplateField);
	        this.doc_url = source["doc_url"];
	        this.supported = source["supported"];
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

export namespace chat {
	
	export class TurnSegmentToolFunction {
	    name: string;
	    arguments: string;
	
	    static createFrom(source: any = {}) {
	        return new TurnSegmentToolFunction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.arguments = source["arguments"];
	    }
	}
	export class TurnSegmentToolCall {
	    id: string;
	    type: string;
	    function: TurnSegmentToolFunction;
	    result?: string;
	
	    static createFrom(source: any = {}) {
	        return new TurnSegmentToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.function = this.convertValues(source["function"], TurnSegmentToolFunction);
	        this.result = source["result"];
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
	export class TurnSegment {
	    type: string;
	    content?: string;
	    toolCalls?: TurnSegmentToolCall[];
	
	    static createFrom(source: any = {}) {
	        return new TurnSegment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.toolCalls = this.convertValues(source["toolCalls"], TurnSegmentToolCall);
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
	export class EnrichedMessage {
	    id: string;
	    conversationId: string;
	    parentId?: string;
	    turnId?: string;
	    role: string;
	    content: string;
	    reasoning?: string;
	    media?: string;
	    toolCalls?: string;
	    toolCallId?: string;
	    promptTokens?: number;
	    completionTokens?: number;
	    totalTokens?: number;
	    cacheReadTokens?: number;
	    cacheWriteTokens?: number;
	    cacheMissTokens?: number;
	    model?: string;
	    source?: string;
	    // Go type: time
	    createdAt: any;
	    timestamp: number;
	    isStreaming: boolean;
	    internal: boolean;
	    turnSegments?: TurnSegment[];
	
	    static createFrom(source: any = {}) {
	        return new EnrichedMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversationId = source["conversationId"];
	        this.parentId = source["parentId"];
	        this.turnId = source["turnId"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning = source["reasoning"];
	        this.media = source["media"];
	        this.toolCalls = source["toolCalls"];
	        this.toolCallId = source["toolCallId"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.cacheReadTokens = source["cacheReadTokens"];
	        this.cacheWriteTokens = source["cacheWriteTokens"];
	        this.cacheMissTokens = source["cacheMissTokens"];
	        this.model = source["model"];
	        this.source = source["source"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.timestamp = source["timestamp"];
	        this.isStreaming = source["isStreaming"];
	        this.internal = source["internal"];
	        this.turnSegments = this.convertValues(source["turnSegments"], TurnSegment);
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
	    originalIndex?: number;
	
	    static createFrom(source: any = {}) {
	        return new MessageNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = this.convertValues(source["message"], EnrichedMessage);
	        this.children = this.convertValues(source["children"], MessageNode);
	        this.level = source["level"];
	        this.childCount = source["childCount"];
	        this.originalIndex = source["originalIndex"];
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
	    id: string;
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
	
	
	export class MessageWindow {
	    scope: string;
	    conversationId: string;
	    threadParentId?: string;
	    nodes: MessageNode[];
	    totalCount: number;
	    startIndex: number;
	    endIndex: number;
	    hasBefore: boolean;
	    hasAfter: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MessageWindow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.conversationId = source["conversationId"];
	        this.threadParentId = source["threadParentId"];
	        this.nodes = this.convertValues(source["nodes"], MessageNode);
	        this.totalCount = source["totalCount"];
	        this.startIndex = source["startIndex"];
	        this.endIndex = source["endIndex"];
	        this.hasBefore = source["hasBefore"];
	        this.hasAfter = source["hasAfter"];
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
	export class MessageWindowRequest {
	    scope: string;
	    conversationId: string;
	    threadParentId?: string;
	    anchor?: string;
	    anchorMessageId?: string;
	    direction: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new MessageWindowRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.conversationId = source["conversationId"];
	        this.threadParentId = source["threadParentId"];
	        this.anchor = source["anchor"];
	        this.anchorMessageId = source["anchorMessageId"];
	        this.direction = source["direction"];
	        this.limit = source["limit"];
	    }
	}
	export class ToolUsageBreakdown {
	    toolName: string;
	    callCount: number;
	    totalPromptTokens: number;
	    totalCompletionTokens: number;
	    totalTokens: number;
	
	    static createFrom(source: any = {}) {
	        return new ToolUsageBreakdown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.toolName = source["toolName"];
	        this.callCount = source["callCount"];
	        this.totalPromptTokens = source["totalPromptTokens"];
	        this.totalCompletionTokens = source["totalCompletionTokens"];
	        this.totalTokens = source["totalTokens"];
	    }
	}
	export class TokenStats {
	    conversationId: string;
	    promptTokens: number;
	    completionTokens: number;
	    totalTokens: number;
	    cacheReadTokens: number;
	    cacheWriteTokens: number;
	    cacheMissTokens: number;
	    messageCount: number;
	    model: string;
	    mostUsedModel: string;
	    contextTokens: number;
	    contextUsage: number;
	    contextLimit: number;
	    isNearLimit: boolean;
	    isCritical: boolean;
	    systemPromptEstimatedTokens: number;
	    summaryTokens: number;
	    messagesInContextCount: number;
	    messagesInContextTokens: number;
	    messagesOutOfContextCount: number;
	    messagesOutOfContextTokens: number;
	    toolsUsedCount: number;
	    toolBreakdown: ToolUsageBreakdown[];
	
	    static createFrom(source: any = {}) {
	        return new TokenStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversationId = source["conversationId"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.cacheReadTokens = source["cacheReadTokens"];
	        this.cacheWriteTokens = source["cacheWriteTokens"];
	        this.cacheMissTokens = source["cacheMissTokens"];
	        this.messageCount = source["messageCount"];
	        this.model = source["model"];
	        this.mostUsedModel = source["mostUsedModel"];
	        this.contextTokens = source["contextTokens"];
	        this.contextUsage = source["contextUsage"];
	        this.contextLimit = source["contextLimit"];
	        this.isNearLimit = source["isNearLimit"];
	        this.isCritical = source["isCritical"];
	        this.systemPromptEstimatedTokens = source["systemPromptEstimatedTokens"];
	        this.summaryTokens = source["summaryTokens"];
	        this.messagesInContextCount = source["messagesInContextCount"];
	        this.messagesInContextTokens = source["messagesInContextTokens"];
	        this.messagesOutOfContextCount = source["messagesOutOfContextCount"];
	        this.messagesOutOfContextTokens = source["messagesOutOfContextTokens"];
	        this.toolsUsedCount = source["toolsUsedCount"];
	        this.toolBreakdown = this.convertValues(source["toolBreakdown"], ToolUsageBreakdown);
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
	    api_key?: string;
	    api_base_url?: string;
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

export namespace contacts {
	
	export class AuthorizedContact {
	    id: string;
	    display_name: string;
	    username?: string;
	    authorized_at: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthorizedContact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.display_name = source["display_name"];
	        this.username = source["username"];
	        this.authorized_at = source["authorized_at"];
	    }
	}

}

export namespace controllers {
	
	export class ChannelInfo {
	    name: string;
	    connected: boolean;
	    contacts: contacts.AuthorizedContact[];
	    maxContacts: number;
	
	    static createFrom(source: any = {}) {
	        return new ChannelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.connected = source["connected"];
	        this.contacts = this.convertValues(source["contacts"], contacts.AuthorizedContact);
	        this.maxContacts = source["maxContacts"];
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
	export class CreateLLMProviderRequest {
	    id: string;
	    name: string;
	    type: string;
	    base_url: string;
	    api_key?: string;
	    default_model?: string;
	    api_format?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateLLMProviderRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.default_model = source["default_model"];
	        this.api_format = source["api_format"];
	    }
	}
	export class CredentialInput {
	    pattern: string;
	    type: string;
	    token?: string;
	    username?: string;
	    password?: string;
	    headerName?: string;
	    headerValue?: string;
	
	    static createFrom(source: any = {}) {
	        return new CredentialInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pattern = source["pattern"];
	        this.type = source["type"];
	        this.token = source["token"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.headerName = source["headerName"];
	        this.headerValue = source["headerValue"];
	    }
	}
	export class CredentialSummary {
	    pattern: string;
	    type: string;
	    masked: string;
	    managed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CredentialSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pattern = source["pattern"];
	        this.type = source["type"];
	        this.masked = source["masked"];
	        this.managed = source["managed"];
	    }
	}
	export class ExternalSourceSuggestion {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ExternalSourceSuggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class LLMSettings {
	    APIKey: string;
	    BaseURL: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.APIKey = source["APIKey"];
	        this.BaseURL = source["BaseURL"];
	    }
	}
	export class SettingsInput {
	    apiKey: string;
	    apiBaseUrl: string;
	    responseTimeout: number;
	    chatParams: config.ModelParams;
	    sttParams: config.STTParams;
	
	    static createFrom(source: any = {}) {
	        return new SettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiKey = source["apiKey"];
	        this.apiBaseUrl = source["apiBaseUrl"];
	        this.responseTimeout = source["responseTimeout"];
	        this.chatParams = this.convertValues(source["chatParams"], config.ModelParams);
	        this.sttParams = this.convertValues(source["sttParams"], config.STTParams);
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
	export class SkillCreateRequest {
	    name: string;
	    version: string;
	    description: string;
	    displayName?: string;
	    author?: string;
	    authorEmail?: string;
	    authorUrl?: string;
	    license?: string;
	    repository?: string;
	    homepage?: string;
	    keywords?: string[];
	    category?: string;
	    subcategory?: string;
	    type?: string;
	    difficulty?: string;
	    audience?: string[];
	    minVersion?: string;
	    maxVersion?: string;
	    platforms?: string[];
	    languages?: string[];
	    frameworks?: string[];
	    disableModelInvocation?: boolean;
	    userInvocable?: boolean;
	    argumentHint?: string;
	    context?: string;
	    agent?: string;
	    model?: string;
	    // Go type: skills
	    filesystem?: any;
	    // Go type: skills
	    network?: any;
	    // Go type: skills
	    tools?: any;
	    // Go type: skills
	    input?: any;
	    // Go type: skills
	    output?: any;
	    // Go type: skills
	    behavior?: any;
	    // Go type: skills
	    triggers?: any;
	    hooks?: any;
	    // Go type: skills
	    dependencies?: any;
	    // Go type: skills
	    mcp?: any;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillCreateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.displayName = source["displayName"];
	        this.author = source["author"];
	        this.authorEmail = source["authorEmail"];
	        this.authorUrl = source["authorUrl"];
	        this.license = source["license"];
	        this.repository = source["repository"];
	        this.homepage = source["homepage"];
	        this.keywords = source["keywords"];
	        this.category = source["category"];
	        this.subcategory = source["subcategory"];
	        this.type = source["type"];
	        this.difficulty = source["difficulty"];
	        this.audience = source["audience"];
	        this.minVersion = source["minVersion"];
	        this.maxVersion = source["maxVersion"];
	        this.platforms = source["platforms"];
	        this.languages = source["languages"];
	        this.frameworks = source["frameworks"];
	        this.disableModelInvocation = source["disableModelInvocation"];
	        this.userInvocable = source["userInvocable"];
	        this.argumentHint = source["argumentHint"];
	        this.context = source["context"];
	        this.agent = source["agent"];
	        this.model = source["model"];
	        this.filesystem = this.convertValues(source["filesystem"], null);
	        this.network = this.convertValues(source["network"], null);
	        this.tools = this.convertValues(source["tools"], null);
	        this.input = this.convertValues(source["input"], null);
	        this.output = this.convertValues(source["output"], null);
	        this.behavior = this.convertValues(source["behavior"], null);
	        this.triggers = this.convertValues(source["triggers"], null);
	        this.hooks = source["hooks"];
	        this.dependencies = this.convertValues(source["dependencies"], null);
	        this.mcp = this.convertValues(source["mcp"], null);
	        this.content = source["content"];
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
	export class TestLLMProviderRequest {
	    type: string;
	    base_url: string;
	    api_key?: string;
	    provider_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new TestLLMProviderRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.provider_id = source["provider_id"];
	    }
	}
	export class ToolInfo {
	    name: string;
	    display_name: string;
	    description: string;
	    source_type: string;
	    source_label: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.source_type = source["source_type"];
	        this.source_label = source["source_label"];
	    }
	}
	export class UpdateLLMProviderRequest {
	    name?: string;
	    type?: string;
	    base_url?: string;
	    api_key?: string;
	    default_model?: string;
	    api_format?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateLLMProviderRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.default_model = source["default_model"];
	        this.api_format = source["api_format"];
	    }
	}

}

export namespace credentials {
	
	export class VaultIntegrityStatus {
	    hasKeyringDEK: boolean;
	    hasMasterWrap: boolean;
	    keychainDekId: string;
	    wrapsDekId: string;
	    ok: boolean;
	    reason?: string;
	    unreadableCredentialIds?: string[];
	
	    static createFrom(source: any = {}) {
	        return new VaultIntegrityStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasKeyringDEK = source["hasKeyringDEK"];
	        this.hasMasterWrap = source["hasMasterWrap"];
	        this.keychainDekId = source["keychainDekId"];
	        this.wrapsDekId = source["wrapsDekId"];
	        this.ok = source["ok"];
	        this.reason = source["reason"];
	        this.unreadableCredentialIds = source["unreadableCredentialIds"];
	    }
	}

}

export namespace database {
	
	export class ChatMessage {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    conversationId: string;
	    parentId?: string;
	    turnId?: string;
	    role: string;
	    content: string;
	    reasoning?: string;
	    media?: string;
	    audio?: string;
	    audioMimeType?: string;
	    toolCalls?: string;
	    toolCallId?: string;
	    promptTokens?: number;
	    completionTokens?: number;
	    totalTokens?: number;
	    cacheReadTokens?: number;
	    cacheWriteTokens?: number;
	    cacheMissTokens?: number;
	    model?: string;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.conversationId = source["conversationId"];
	        this.parentId = source["parentId"];
	        this.turnId = source["turnId"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning = source["reasoning"];
	        this.media = source["media"];
	        this.audio = source["audio"];
	        this.audioMimeType = source["audioMimeType"];
	        this.toolCalls = source["toolCalls"];
	        this.toolCallId = source["toolCallId"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.cacheReadTokens = source["cacheReadTokens"];
	        this.cacheWriteTokens = source["cacheWriteTokens"];
	        this.cacheMissTokens = source["cacheMissTokens"];
	        this.model = source["model"];
	        this.source = source["source"];
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
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    userId?: string;
	    title: string;
	    channel?: string;
	    contact_id?: string;
	    messages?: ChatMessage[];
	    message_count: number;
	    kind?: string;
	    parentConversationId?: string;
	    latestStatus?: string;
	    summary?: string;
	    summary_up_to_message_id?: string;
	    summarizing_in_progress?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.userId = source["userId"];
	        this.title = source["title"];
	        this.channel = source["channel"];
	        this.contact_id = source["contact_id"];
	        this.messages = this.convertValues(source["messages"], ChatMessage);
	        this.message_count = source["message_count"];
	        this.kind = source["kind"];
	        this.parentConversationId = source["parentConversationId"];
	        this.latestStatus = source["latestStatus"];
	        this.summary = source["summary"];
	        this.summary_up_to_message_id = source["summary_up_to_message_id"];
	        this.summarizing_in_progress = source["summarizing_in_progress"];
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
	export class CustomAction {
	    id: string;
	    label: string;
	    icon?: string;
	    surfaces?: string[];
	    event?: string;
	    payload_template?: string;
	    link?: string;
	    when?: string;
	    danger?: boolean;
	    confirm?: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.surfaces = source["surfaces"];
	        this.event = source["event"];
	        this.payload_template = source["payload_template"];
	        this.link = source["link"];
	        this.when = source["when"];
	        this.danger = source["danger"];
	        this.confirm = source["confirm"];
	    }
	}
	export class MemoryRecord {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    userId?: string;
	    content: string;
	    summary?: string;
	    loadPolicy: string;
	    archivedFromPolicy?: string;
	    kind: string;
	    scope: string;
	    scopeRef?: string;
	    tags?: string;
	    importance: number;
	    confidence: number;
	    sourceType?: string;
	    sourceId?: string;
	    // Go type: time
	    lastUsedAt?: any;
	    // Go type: time
	    expiresAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new MemoryRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.userId = source["userId"];
	        this.content = source["content"];
	        this.summary = source["summary"];
	        this.loadPolicy = source["loadPolicy"];
	        this.archivedFromPolicy = source["archivedFromPolicy"];
	        this.kind = source["kind"];
	        this.scope = source["scope"];
	        this.scopeRef = source["scopeRef"];
	        this.tags = source["tags"];
	        this.importance = source["importance"];
	        this.confidence = source["confidence"];
	        this.sourceType = source["sourceType"];
	        this.sourceId = source["sourceId"];
	        this.lastUsedAt = this.convertValues(source["lastUsedAt"], null);
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
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
	export class MessageSearchResult {
	    conversation_id: string;
	    conversation_title: string;
	    message_id: string;
	    role: string;
	    snippet: string;
	    rank: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new MessageSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.conversation_title = source["conversation_title"];
	        this.message_id = source["message_id"];
	        this.role = source["role"];
	        this.snippet = source["snippet"];
	        this.rank = source["rank"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class Session {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    userId: string;
	    // Go type: time
	    expiresAt: any;
	    // Go type: time
	    lastUsedAt?: any;
	    // Go type: time
	    revokedAt?: any;
	    clientLabel?: string;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.userId = source["userId"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	        this.lastUsedAt = this.convertValues(source["lastUsedAt"], null);
	        this.revokedAt = this.convertValues(source["revokedAt"], null);
	        this.clientLabel = source["clientLabel"];
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
	export class TaskNote {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    userId?: string;
	    task_id: string;
	    type: number;
	    content: string;
	    author_name?: string;
	    author_id?: string;
	    source?: string;
	    external_id?: string;
	    external_parent_id?: string;
	    // Go type: time
	    external_updated_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new TaskNote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.userId = source["userId"];
	        this.task_id = source["task_id"];
	        this.type = source["type"];
	        this.content = source["content"];
	        this.author_name = source["author_name"];
	        this.author_id = source["author_id"];
	        this.source = source["source"];
	        this.external_id = source["external_id"];
	        this.external_parent_id = source["external_parent_id"];
	        this.external_updated_at = this.convertValues(source["external_updated_at"], null);
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
	export class TaskListWorkflow {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    task_list_id: string;
	    statuses: string;
	    allowed_transitions: string;
	    initial_status_id: number;
	    task_list?: TaskList;
	
	    static createFrom(source: any = {}) {
	        return new TaskListWorkflow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.task_list_id = source["task_list_id"];
	        this.statuses = source["statuses"];
	        this.allowed_transitions = source["allowed_transitions"];
	        this.initial_status_id = source["initial_status_id"];
	        this.task_list = this.convertValues(source["task_list"], TaskList);
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
	export class TaskList {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    userId?: string;
	    title: string;
	    slug?: string;
	    description: string;
	    preferred_view_mode: string;
	    validation_policy?: string;
	    custom_actions?: string;
	    conversation_id?: string;
	    workflow?: TaskListWorkflow;
	    tasks?: Task[];
	
	    static createFrom(source: any = {}) {
	        return new TaskList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.userId = source["userId"];
	        this.title = source["title"];
	        this.slug = source["slug"];
	        this.description = source["description"];
	        this.preferred_view_mode = source["preferred_view_mode"];
	        this.validation_policy = source["validation_policy"];
	        this.custom_actions = source["custom_actions"];
	        this.conversation_id = source["conversation_id"];
	        this.workflow = this.convertValues(source["workflow"], TaskListWorkflow);
	        this.tasks = this.convertValues(source["tasks"], Task);
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
	export class Task {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    task_list_id: string;
	    title: string;
	    description: string;
	    code?: string;
	    link?: string;
	    status_id: number;
	    parent_id?: string;
	    order: number;
	    assignee_name?: string;
	    assignee_id?: string;
	    creator_name?: string;
	    creator_id?: string;
	    // Go type: time
	    due_date?: any;
	    // Go type: time
	    completed_at?: any;
	    conversation_id?: string;
	    task_list?: TaskList;
	    parent?: Task;
	    subtasks?: Task[];
	    notes?: TaskNote[];
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.task_list_id = source["task_list_id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.code = source["code"];
	        this.link = source["link"];
	        this.status_id = source["status_id"];
	        this.parent_id = source["parent_id"];
	        this.order = source["order"];
	        this.assignee_name = source["assignee_name"];
	        this.assignee_id = source["assignee_id"];
	        this.creator_name = source["creator_name"];
	        this.creator_id = source["creator_id"];
	        this.due_date = this.convertValues(source["due_date"], null);
	        this.completed_at = this.convertValues(source["completed_at"], null);
	        this.conversation_id = source["conversation_id"];
	        this.task_list = this.convertValues(source["task_list"], TaskList);
	        this.parent = this.convertValues(source["parent"], Task);
	        this.subtasks = this.convertValues(source["subtasks"], Task);
	        this.notes = this.convertValues(source["notes"], TaskNote);
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
	
	export class TaskListCustomActions {
	    actions?: CustomAction[];
	
	    static createFrom(source: any = {}) {
	        return new TaskListCustomActions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.actions = this.convertValues(source["actions"], CustomAction);
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
	
	export class TaskListWorkflowStatus {
	    id: number;
	    order: number;
	    label: string;
	    color: string;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskListWorkflowStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.order = source["order"];
	        this.label = source["label"];
	        this.color = source["color"];
	        this.icon = source["icon"];
	    }
	}
	
	export class User {
	    id: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    username: string;
	    displayName: string;
	    role: string;
	    isActive: boolean;
	    // Go type: time
	    lastLoginAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.username = source["username"];
	        this.displayName = source["displayName"];
	        this.role = source["role"];
	        this.isActive = source["isActive"];
	        this.lastLoginAt = this.convertValues(source["lastLoginAt"], null);
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

export namespace jobs {
	
	export class CatalogEntry {
	    id?: string;
	    mcp_server_id?: string;
	    name: string;
	    description: string;
	    schema: number[];
	    source: string;
	    origin?: string;
	    risk?: string;
	    availability_status?: string;
	    availability_reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new CatalogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.mcp_server_id = source["mcp_server_id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.schema = source["schema"];
	        this.source = source["source"];
	        this.origin = source["origin"];
	        this.risk = source["risk"];
	        this.availability_status = source["availability_status"];
	        this.availability_reason = source["availability_reason"];
	    }
	}
	export class DryRunConfig {
	    enabled?: boolean;
	    mock_output?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new DryRunConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.mock_output = source["mock_output"];
	    }
	}
	export class TriggerInfo {
	    type: string;
	    // Go type: time
	    at: any;
	    event?: string;
	    expression?: string;
	    every?: string;
	    keys?: string;
	    when?: string;
	    data?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new TriggerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.at = this.convertValues(source["at"], null);
	        this.event = source["event"];
	        this.expression = source["expression"];
	        this.every = source["every"];
	        this.keys = source["keys"];
	        this.when = source["when"];
	        this.data = source["data"];
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
	export class RunLog {
	    run_id: string;
	    job_id: string;
	    tool_name?: string;
	    trigger: TriggerInfo;
	    status: string;
	    // Go type: time
	    started_at: any;
	    // Go type: time
	    completed_at?: any;
	    duration?: string;
	    resolved_inputs?: Record<string, any>;
	    output?: Record<string, any>;
	    output_size?: number;
	    error?: string;
	    retry_count?: number;
	    events_emitted?: string[];
	    is_dry_run?: boolean;
	    replayable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run_id = source["run_id"];
	        this.job_id = source["job_id"];
	        this.tool_name = source["tool_name"];
	        this.trigger = this.convertValues(source["trigger"], TriggerInfo);
	        this.status = source["status"];
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.completed_at = this.convertValues(source["completed_at"], null);
	        this.duration = source["duration"];
	        this.resolved_inputs = source["resolved_inputs"];
	        this.output = source["output"];
	        this.output_size = source["output_size"];
	        this.error = source["error"];
	        this.retry_count = source["retry_count"];
	        this.events_emitted = source["events_emitted"];
	        this.is_dry_run = source["is_dry_run"];
	        this.replayable = source["replayable"];
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
	export class DryRunResult {
	    success: boolean;
	    output?: Record<string, any>;
	    error?: string;
	    run_log?: RunLog;
	
	    static createFrom(source: any = {}) {
	        return new DryRunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.output = source["output"];
	        this.error = source["error"];
	        this.run_log = this.convertValues(source["run_log"], RunLog);
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
	export class ErrorPolicy {
	    strategy?: string;
	    max_retries?: number;
	    retry_delay?: string;
	    backoff?: string;
	    on_exhausted?: string;
	    notify_channels?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ErrorPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.strategy = source["strategy"];
	        this.max_retries = source["max_retries"];
	        this.retry_delay = source["retry_delay"];
	        this.backoff = source["backoff"];
	        this.on_exhausted = source["on_exhausted"];
	        this.notify_channels = source["notify_channels"];
	    }
	}
	export class EventEntry {
	    id?: string;
	    // Go type: time
	    timestamp: any;
	    type: string;
	    job_id: string;
	    run_id?: string;
	    event?: string;
	    message?: string;
	    data?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new EventEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.type = source["type"];
	        this.job_id = source["job_id"];
	        this.run_id = source["run_id"];
	        this.event = source["event"];
	        this.message = source["message"];
	        this.data = source["data"];
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
	export class PayloadFilter {
	    include?: string[];
	    exclude?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PayloadFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.include = source["include"];
	        this.exclude = source["exclude"];
	    }
	}
	export class EventsConfig {
	    on_success?: string;
	    on_failure?: string;
	    emit_when?: string;
	    for_each?: string;
	    payload_template?: string;
	    payload_filter?: PayloadFilter;
	
	    static createFrom(source: any = {}) {
	        return new EventsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.on_success = source["on_success"];
	        this.on_failure = source["on_failure"];
	        this.emit_when = source["emit_when"];
	        this.for_each = source["for_each"];
	        this.payload_template = source["payload_template"];
	        this.payload_filter = this.convertValues(source["payload_filter"], PayloadFilter);
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
	export class Metadata {
	    created_at?: string;
	    created_by?: string;
	    updated_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new Metadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.created_at = source["created_at"];
	        this.created_by = source["created_by"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class OutputConfig {
	    schema?: number[];
	    map?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new OutputConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.map = source["map"];
	    }
	}
	export class Trigger {
	    type: string;
	    expression?: string;
	    every?: string;
	    listen?: string;
	    keys?: string;
	    path?: string;
	    when?: string;
	
	    static createFrom(source: any = {}) {
	        return new Trigger(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.expression = source["expression"];
	        this.every = source["every"];
	        this.listen = source["listen"];
	        this.keys = source["keys"];
	        this.path = source["path"];
	        this.when = source["when"];
	    }
	}
	export class Job {
	    id: string;
	    name: string;
	    description: string;
	    enabled: boolean;
	    pipeline?: string;
	    tags?: string[];
	    triggers: Trigger[];
	    tool: string;
	    inputs?: Record<string, any>;
	    output?: OutputConfig;
	    events?: EventsConfig;
	    error_policy?: ErrorPolicy;
	    max_runs_per_hour?: number;
	    dry_run?: DryRunConfig;
	    metadata?: Metadata;
	    last_run?: RunLog;
	    status: string;
	    pipeline_enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
	        this.pipeline = source["pipeline"];
	        this.tags = source["tags"];
	        this.triggers = this.convertValues(source["triggers"], Trigger);
	        this.tool = source["tool"];
	        this.inputs = source["inputs"];
	        this.output = this.convertValues(source["output"], OutputConfig);
	        this.events = this.convertValues(source["events"], EventsConfig);
	        this.error_policy = this.convertValues(source["error_policy"], ErrorPolicy);
	        this.max_runs_per_hour = source["max_runs_per_hour"];
	        this.dry_run = this.convertValues(source["dry_run"], DryRunConfig);
	        this.metadata = this.convertValues(source["metadata"], Metadata);
	        this.last_run = this.convertValues(source["last_run"], RunLog);
	        this.status = source["status"];
	        this.pipeline_enabled = source["pipeline_enabled"];
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
	export class JobInfo {
	    id: string;
	    name: string;
	    description: string;
	    enabled: boolean;
	    effective_enabled: boolean;
	    pipeline_enabled: boolean;
	    pipeline?: string;
	    tags?: string[];
	    tool: string;
	    status: string;
	    triggers: Trigger[];
	    last_run?: RunLog;
	
	    static createFrom(source: any = {}) {
	        return new JobInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
	        this.effective_enabled = source["effective_enabled"];
	        this.pipeline_enabled = source["pipeline_enabled"];
	        this.pipeline = source["pipeline"];
	        this.tags = source["tags"];
	        this.tool = source["tool"];
	        this.status = source["status"];
	        this.triggers = this.convertValues(source["triggers"], Trigger);
	        this.last_run = this.convertValues(source["last_run"], RunLog);
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
	
	
	
	export class PipelineInfo {
	    name: string;
	    jobs: JobInfo[];
	
	    static createFrom(source: any = {}) {
	        return new PipelineInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.jobs = this.convertValues(source["jobs"], JobInfo);
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
	export class RunEvent {
	    id?: string;
	    run_id: string;
	    sequence: number;
	    // Go type: time
	    timestamp: any;
	    type: string;
	    message?: string;
	    data?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new RunEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.run_id = source["run_id"];
	        this.sequence = source["sequence"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.type = source["type"];
	        this.message = source["message"];
	        this.data = source["data"];
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
	
	export class TestToolResult {
	    success: boolean;
	    output?: Record<string, any>;
	    error?: string;
	    duration?: string;
	    blocked?: boolean;
	    origin?: string;
	    mcp_server_id?: string;
	    tool_name?: string;
	    tool_catalog_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new TestToolResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.output = source["output"];
	        this.error = source["error"];
	        this.duration = source["duration"];
	        this.blocked = source["blocked"];
	        this.origin = source["origin"];
	        this.mcp_server_id = source["mcp_server_id"];
	        this.tool_name = source["tool_name"];
	        this.tool_catalog_id = source["tool_catalog_id"];
	    }
	}
	

}

export namespace llm {
	
	export class ChatParams {
	    model: string;
	    maxTokens: number;
	    maxTokensMode?: string;
	    temperature: number;
	    topP?: number;
	    reasoningEffort?: string;
	    profileSlug?: string;
	    allowAssistantPrefill?: boolean;
	    continueViaUserMessage?: boolean;
	    maxAgenticIterations?: number;
	    responseTimeout?: number;
	    contextWindow?: number;
	    tabType?: string;
	    activeFilePath?: string;
	    surfaceStateJson?: string;
	    surfaceContextJson?: string;
	    surfaceSessionKey?: string;
	    surfaceId?: string;
	    surfaceType?: string;
	    surfaceTabId?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.maxTokens = source["maxTokens"];
	        this.maxTokensMode = source["maxTokensMode"];
	        this.temperature = source["temperature"];
	        this.topP = source["topP"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.profileSlug = source["profileSlug"];
	        this.allowAssistantPrefill = source["allowAssistantPrefill"];
	        this.continueViaUserMessage = source["continueViaUserMessage"];
	        this.maxAgenticIterations = source["maxAgenticIterations"];
	        this.responseTimeout = source["responseTimeout"];
	        this.contextWindow = source["contextWindow"];
	        this.tabType = source["tabType"];
	        this.activeFilePath = source["activeFilePath"];
	        this.surfaceStateJson = source["surfaceStateJson"];
	        this.surfaceContextJson = source["surfaceContextJson"];
	        this.surfaceSessionKey = source["surfaceSessionKey"];
	        this.surfaceId = source["surfaceId"];
	        this.surfaceType = source["surfaceType"];
	        this.surfaceTabId = source["surfaceTabId"];
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
	export class FunctionDefinition {
	    name: string;
	    description: string;
	    parameters: number[];
	
	    static createFrom(source: any = {}) {
	        return new FunctionDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.parameters = source["parameters"];
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
	    thinking?: string;
	    tool_calls?: ToolCall[];
	    tool_call_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.thinking = source["thinking"];
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
	export class ToolDefinition {
	    type: string;
	    function: FunctionDefinition;
	
	    static createFrom(source: any = {}) {
	        return new ToolDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.function = this.convertValues(source["function"], FunctionDefinition);
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
	export class NativeMCPAdapterFallback {
	    Streamer: any;
	    ToolDefs: ToolDefinition[];
	
	    static createFrom(source: any = {}) {
	        return new NativeMCPAdapterFallback(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Streamer = source["Streamer"];
	        this.ToolDefs = this.convertValues(source["ToolDefs"], ToolDefinition);
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
	export class ProviderConfig {
	    id: string;
	    name: string;
	    type: string;
	    api_format?: string;
	    base_url: string;
	    model?: string;
	    default_model?: string;
	    is_default?: boolean;
	    timeout?: number;
	    headers?: Record<string, string>;
	    credential_pattern?: string;
	    auth_mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.api_format = source["api_format"];
	        this.base_url = source["base_url"];
	        this.model = source["model"];
	        this.default_model = source["default_model"];
	        this.is_default = source["is_default"];
	        this.timeout = source["timeout"];
	        this.headers = source["headers"];
	        this.credential_pattern = source["credential_pattern"];
	        this.auth_mode = source["auth_mode"];
	    }
	}
	

}

export namespace mcp {
	
	export class MCPPromptArgument {
	    name: string;
	    description: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MCPPromptArgument(source);
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
	    arguments: MCPPromptArgument[];
	    serverSlug: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPPromptInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.arguments = this.convertValues(source["arguments"], MCPPromptArgument);
	        this.serverSlug = source["serverSlug"];
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
	export class MCPResourceInfo {
	    uri: string;
	    name: string;
	    description: string;
	    mimeType: string;
	    serverSlug: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPResourceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.mimeType = source["mimeType"];
	        this.serverSlug = source["serverSlug"];
	    }
	}
	export class MCPServerLog {
	    id?: string;
	    serverId?: string;
	    slug?: string;
	    // Go type: time
	    timestamp: any;
	    type: string;
	    message?: string;
	    data?: number[];
	    // Go type: time
	    createdAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new MCPServerLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.slug = source["slug"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.type = source["type"];
	        this.message = source["message"];
	        this.data = source["data"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class MCPToolInfo {
	    name: string;
	    fullName: string;
	    description: string;
	    schema: number[];
	    serverSlug: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPToolInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.fullName = source["fullName"];
	        this.description = source["description"];
	        this.schema = source["schema"];
	        this.serverSlug = source["serverSlug"];
	    }
	}
	export class OAuthDiscoveryResult {
	    found: boolean;
	    authType: string;
	    authUrl: string;
	    tokenUrl: string;
	    scopes: string[];
	    clientId?: string;
	    registrationUrl?: string;
	    resourceName?: string;
	    supportsPkce: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new OAuthDiscoveryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.authType = source["authType"];
	        this.authUrl = source["authUrl"];
	        this.tokenUrl = source["tokenUrl"];
	        this.scopes = source["scopes"];
	        this.clientId = source["clientId"];
	        this.registrationUrl = source["registrationUrl"];
	        this.resourceName = source["resourceName"];
	        this.supportsPkce = source["supportsPkce"];
	        this.error = source["error"];
	    }
	}
	export class Root {
	    uri: string;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new Root(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.name = source["name"];
	    }
	}
	export class ServerConfig {
	    id?: string;
	    user_id?: string;
	    slug?: string;
	    name: string;
	    description?: string;
	    transport: string;
	    command?: string;
	    args?: string[];
	    env?: Record<string, string>;
	    url?: string;
	    auth_type?: string;
	    oauth2_client_id?: string;
	    oauth2_auth_url?: string;
	    oauth2_token_url?: string;
	    oauth2_scopes?: string[];
	    oauth2_callback_port?: number;
	    oauth2_callback_host?: string;
	    oauth2_registration_url?: string;
	    oauth2_device_auth_url?: string;
	    disable_sse?: boolean;
	    prefer_bridge?: boolean;
	    enabled: boolean;
	    auto_connect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.user_id = source["user_id"];
	        this.slug = source["slug"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.transport = source["transport"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.url = source["url"];
	        this.auth_type = source["auth_type"];
	        this.oauth2_client_id = source["oauth2_client_id"];
	        this.oauth2_auth_url = source["oauth2_auth_url"];
	        this.oauth2_token_url = source["oauth2_token_url"];
	        this.oauth2_scopes = source["oauth2_scopes"];
	        this.oauth2_callback_port = source["oauth2_callback_port"];
	        this.oauth2_callback_host = source["oauth2_callback_host"];
	        this.oauth2_registration_url = source["oauth2_registration_url"];
	        this.oauth2_device_auth_url = source["oauth2_device_auth_url"];
	        this.disable_sse = source["disable_sse"];
	        this.prefer_bridge = source["prefer_bridge"];
	        this.enabled = source["enabled"];
	        this.auto_connect = source["auto_connect"];
	    }
	}
	export class ServerInfo {
	    id?: string;
	    slug: string;
	    name: string;
	    description?: string;
	    transport: string;
	    status: string;
	    error?: string;
	    toolCount: number;
	    tools: MCPToolInfo[];
	    resourceCount: number;
	    resources: MCPResourceInfo[];
	    promptCount: number;
	    prompts: MCPPromptInfo[];
	    enabled: boolean;
	    autoConnect: boolean;
	    connectedAt?: string;
	    lastPing?: string;
	    command?: string;
	    args?: string[];
	    url?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.slug = source["slug"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.transport = source["transport"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.toolCount = source["toolCount"];
	        this.tools = this.convertValues(source["tools"], MCPToolInfo);
	        this.resourceCount = source["resourceCount"];
	        this.resources = this.convertValues(source["resources"], MCPResourceInfo);
	        this.promptCount = source["promptCount"];
	        this.prompts = this.convertValues(source["prompts"], MCPPromptInfo);
	        this.enabled = source["enabled"];
	        this.autoConnect = source["autoConnect"];
	        this.connectedAt = source["connectedAt"];
	        this.lastPing = source["lastPing"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.url = source["url"];
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

export namespace memory {
	
	export class Filter {
	    query?: string;
	    loadPolicies?: string[];
	    kinds?: string[];
	    scopes?: string[];
	    tags?: string[];
	    includeArchived?: boolean;
	    limit?: number;
	    offset?: number;
	
	    static createFrom(source: any = {}) {
	        return new Filter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.loadPolicies = source["loadPolicies"];
	        this.kinds = source["kinds"];
	        this.scopes = source["scopes"];
	        this.tags = source["tags"];
	        this.includeArchived = source["includeArchived"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class ListResult {
	    records: database.MemoryRecord[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new ListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = this.convertValues(source["records"], database.MemoryRecord);
	        this.total = source["total"];
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
	export class PolicySummary {
	    core: number;
	    pinned: number;
	    auto: number;
	    retrievable: number;
	    archived: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new PolicySummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.core = source["core"];
	        this.pinned = source["pinned"];
	        this.auto = source["auto"];
	        this.retrievable = source["retrievable"];
	        this.archived = source["archived"];
	        this.total = source["total"];
	    }
	}
	export class RecordInput {
	    content: string;
	    summary?: string;
	    loadPolicy?: string;
	    kind?: string;
	    scope?: string;
	    scopeRef?: string;
	    tags?: string[];
	    importance?: number;
	    confidence?: number;
	    sourceType?: string;
	    sourceId?: string;
	    // Go type: time
	    expiresAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new RecordInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.summary = source["summary"];
	        this.loadPolicy = source["loadPolicy"];
	        this.kind = source["kind"];
	        this.scope = source["scope"];
	        this.scopeRef = source["scopeRef"];
	        this.tags = source["tags"];
	        this.importance = source["importance"];
	        this.confidence = source["confidence"];
	        this.sourceType = source["sourceType"];
	        this.sourceId = source["sourceId"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
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

export namespace portability {
	
	export class ContentExportOptions {
	    includeTimestamps: boolean;
	    includeReasoning: boolean;
	    includeMetadata: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContentExportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.includeTimestamps = source["includeTimestamps"];
	        this.includeReasoning = source["includeReasoning"];
	        this.includeMetadata = source["includeMetadata"];
	    }
	}
	export class ExportRequest {
	    all: boolean;
	    explicitSelection?: boolean;
	    conversationIds?: string[];
	    providerIds?: string[];
	    profileSlugs?: string[];
	    skillSlugs?: string[];
	    allowlistSlugs?: string[];
	    mcpServerSlugs?: string[];
	    jobIds?: string[];
	    taskListIds?: string[];
	    memoryRecordIds?: string[];
	    channelNames?: string[];
	    includeContacts: boolean;
	    includeWorkspace: boolean;
	    includeAudio: boolean;
	    includeCredentials: boolean;
	    credentialExportPassword?: string;
	    outputFormat?: string;
	    includeTimestamps?: boolean;
	    includeReasoning?: boolean;
	    includeMetadata?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.all = source["all"];
	        this.explicitSelection = source["explicitSelection"];
	        this.conversationIds = source["conversationIds"];
	        this.providerIds = source["providerIds"];
	        this.profileSlugs = source["profileSlugs"];
	        this.skillSlugs = source["skillSlugs"];
	        this.allowlistSlugs = source["allowlistSlugs"];
	        this.mcpServerSlugs = source["mcpServerSlugs"];
	        this.jobIds = source["jobIds"];
	        this.taskListIds = source["taskListIds"];
	        this.memoryRecordIds = source["memoryRecordIds"];
	        this.channelNames = source["channelNames"];
	        this.includeContacts = source["includeContacts"];
	        this.includeWorkspace = source["includeWorkspace"];
	        this.includeAudio = source["includeAudio"];
	        this.includeCredentials = source["includeCredentials"];
	        this.credentialExportPassword = source["credentialExportPassword"];
	        this.outputFormat = source["outputFormat"];
	        this.includeTimestamps = source["includeTimestamps"];
	        this.includeReasoning = source["includeReasoning"];
	        this.includeMetadata = source["includeMetadata"];
	    }
	}
	export class ImportConflict {
	    resourceType: string;
	    identifier: string;
	    reason: string;
	    supportedStrategies?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportConflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resourceType = source["resourceType"];
	        this.identifier = source["identifier"];
	        this.reason = source["reason"];
	        this.supportedStrategies = source["supportedStrategies"];
	    }
	}
	export class ImportAnalysis {
	    version: number;
	    appVersion?: string;
	    conversationCount: number;
	    messageCount: number;
	    providerCount: number;
	    mcpServerCount: number;
	    taskListCount: number;
	    taskCount: number;
	    taskNoteCount: number;
	    memoryRecordCount: number;
	    includesCredentials: boolean;
	    requiresCredentialPassword: boolean;
	    credentialCount: number;
	    conflictCount: number;
	    conversationConflicts?: ImportConflict[];
	    providerConflicts?: ImportConflict[];
	    mcpServerConflicts?: ImportConflict[];
	    taskListConflicts?: ImportConflict[];
	    credentialConflicts?: ImportConflict[];
	    unsupportedResourceTypes?: string[];
	    warnings?: string[];
	    credentialAnalysisError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.appVersion = source["appVersion"];
	        this.conversationCount = source["conversationCount"];
	        this.messageCount = source["messageCount"];
	        this.providerCount = source["providerCount"];
	        this.mcpServerCount = source["mcpServerCount"];
	        this.taskListCount = source["taskListCount"];
	        this.taskCount = source["taskCount"];
	        this.taskNoteCount = source["taskNoteCount"];
	        this.memoryRecordCount = source["memoryRecordCount"];
	        this.includesCredentials = source["includesCredentials"];
	        this.requiresCredentialPassword = source["requiresCredentialPassword"];
	        this.credentialCount = source["credentialCount"];
	        this.conflictCount = source["conflictCount"];
	        this.conversationConflicts = this.convertValues(source["conversationConflicts"], ImportConflict);
	        this.providerConflicts = this.convertValues(source["providerConflicts"], ImportConflict);
	        this.mcpServerConflicts = this.convertValues(source["mcpServerConflicts"], ImportConflict);
	        this.taskListConflicts = this.convertValues(source["taskListConflicts"], ImportConflict);
	        this.credentialConflicts = this.convertValues(source["credentialConflicts"], ImportConflict);
	        this.unsupportedResourceTypes = source["unsupportedResourceTypes"];
	        this.warnings = source["warnings"];
	        this.credentialAnalysisError = source["credentialAnalysisError"];
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
	
	export class ImportResolution {
	    resourceType: string;
	    identifier: string;
	    strategy: string;
	    renameValue?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResolution(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resourceType = source["resourceType"];
	        this.identifier = source["identifier"];
	        this.strategy = source["strategy"];
	        this.renameValue = source["renameValue"];
	    }
	}
	export class ImportRequest {
	    jsonData: string;
	    credentialExportPassword?: string;
	    resolutions?: ImportResolution[];
	
	    static createFrom(source: any = {}) {
	        return new ImportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jsonData = source["jsonData"];
	        this.credentialExportPassword = source["credentialExportPassword"];
	        this.resolutions = this.convertValues(source["resolutions"], ImportResolution);
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
	
	export class ImportResult {
	    success: boolean;
	    imported: number;
	    skipped: number;
	    failed: number;
	    skippedEmptyConversations: number;
	    skippedConversationConflict: number;
	    skippedProviderConflict: number;
	    skippedMcpServerConflict: number;
	    skippedTaskListConflict: number;
	    skippedCredentialConflict: number;
	    skippedOther: number;
	    unsupportedResourceTypes?: string[];
	    warnings?: string[];
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
	        this.failed = source["failed"];
	        this.skippedEmptyConversations = source["skippedEmptyConversations"];
	        this.skippedConversationConflict = source["skippedConversationConflict"];
	        this.skippedProviderConflict = source["skippedProviderConflict"];
	        this.skippedMcpServerConflict = source["skippedMcpServerConflict"];
	        this.skippedTaskListConflict = source["skippedTaskListConflict"];
	        this.skippedCredentialConflict = source["skippedCredentialConflict"];
	        this.skippedOther = source["skippedOther"];
	        this.unsupportedResourceTypes = source["unsupportedResourceTypes"];
	        this.warnings = source["warnings"];
	        this.errors = source["errors"];
	        this.message = source["message"];
	    }
	}

}

export namespace profiles {
	
	export class ChannelsConfig {
	    response_mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChannelsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.response_mode = source["response_mode"];
	    }
	}
	export class ChatConfig {
	    llm_provider: string;
	    model?: string;
	    temperature: number;
	    max_tokens: number;
	    max_tokens_mode?: string;
	    context_window?: number;
	    max_context_messages?: number;
	    min_context_messages?: number;
	    top_p: number;
	    response_timeout: number;
	    reasoning_effort?: string;
	    enabled_tools: string[];
	    enabled_skills: string[];
	    disable_tools?: boolean;
	    disable_skills?: boolean;
	    disable_on_demand_skills?: boolean;
	    command_allowlist?: string;
	    native_mcp?: boolean;
	    max_agentic_iterations?: number;
	    streaming_recovery_enabled?: boolean;
	    streaming_recovery_max_attempts?: number;
	    streaming_recovery_show_continue?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.llm_provider = source["llm_provider"];
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.max_tokens = source["max_tokens"];
	        this.max_tokens_mode = source["max_tokens_mode"];
	        this.context_window = source["context_window"];
	        this.max_context_messages = source["max_context_messages"];
	        this.min_context_messages = source["min_context_messages"];
	        this.top_p = source["top_p"];
	        this.response_timeout = source["response_timeout"];
	        this.reasoning_effort = source["reasoning_effort"];
	        this.enabled_tools = source["enabled_tools"];
	        this.enabled_skills = source["enabled_skills"];
	        this.disable_tools = source["disable_tools"];
	        this.disable_skills = source["disable_skills"];
	        this.disable_on_demand_skills = source["disable_on_demand_skills"];
	        this.command_allowlist = source["command_allowlist"];
	        this.native_mcp = source["native_mcp"];
	        this.max_agentic_iterations = source["max_agentic_iterations"];
	        this.streaming_recovery_enabled = source["streaming_recovery_enabled"];
	        this.streaming_recovery_max_attempts = source["streaming_recovery_max_attempts"];
	        this.streaming_recovery_show_continue = source["streaming_recovery_show_continue"];
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
	export class InputConfig {
	    enabled: boolean;
	    stt_provider: string;
	    llm_provider_id?: string;
	    stt_model?: string;
	    language: string;
	    feedback_sounds: boolean;
	    triggers?: TriggerConfig[];
	
	    static createFrom(source: any = {}) {
	        return new InputConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.stt_provider = source["stt_provider"];
	        this.llm_provider_id = source["llm_provider_id"];
	        this.stt_model = source["stt_model"];
	        this.language = source["language"];
	        this.feedback_sounds = source["feedback_sounds"];
	        this.triggers = this.convertValues(source["triggers"], TriggerConfig);
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
	export class MediaSupport {
	    audio?: boolean;
	    image?: boolean;
	    document?: boolean;
	    video?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MediaSupport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audio = source["audio"];
	        this.image = source["image"];
	        this.document = source["document"];
	        this.video = source["video"];
	    }
	}
	export class VoiceRoleConfig {
	    enabled: boolean;
	    provider: string;
	    llm_provider_id?: string;
	    voice_id?: string;
	    model?: string;
	    selection_mode?: string;
	    rate: number;
	    pitch: number;
	    volume: number;
	
	    static createFrom(source: any = {}) {
	        return new VoiceRoleConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.provider = source["provider"];
	        this.llm_provider_id = source["llm_provider_id"];
	        this.voice_id = source["voice_id"];
	        this.model = source["model"];
	        this.selection_mode = source["selection_mode"];
	        this.rate = source["rate"];
	        this.pitch = source["pitch"];
	        this.volume = source["volume"];
	    }
	}
	export class VoiceConfig {
	    assistant: VoiceRoleConfig;
	    user: VoiceRoleConfig;
	    system: VoiceRoleConfig;
	
	    static createFrom(source: any = {}) {
	        return new VoiceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assistant = this.convertValues(source["assistant"], VoiceRoleConfig);
	        this.user = this.convertValues(source["user"], VoiceRoleConfig);
	        this.system = this.convertValues(source["system"], VoiceRoleConfig);
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
	export class Profile {
	    _builtin_version?: string;
	    name: string;
	    description?: string;
	    icon?: string;
	    active?: boolean;
	    chat: ChatConfig;
	    voice: VoiceConfig;
	    input: InputConfig;
	    channels?: ChannelsConfig;
	    media_support?: MediaSupport;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._builtin_version = source["_builtin_version"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.active = source["active"];
	        this.chat = this.convertValues(source["chat"], ChatConfig);
	        this.voice = this.convertValues(source["voice"], VoiceConfig);
	        this.input = this.convertValues(source["input"], InputConfig);
	        this.channels = this.convertValues(source["channels"], ChannelsConfig);
	        this.media_support = this.convertValues(source["media_support"], MediaSupport);
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

export namespace skills {
	
	export class MCPToolDef {
	    name: string;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPToolDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class MCPServerConfig {
	    command?: string;
	    args?: string[];
	    env?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new MCPServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	    }
	}
	export class MCPConfig {
	    // Go type: MCPServerConfig
	    server?: any;
	    tools?: MCPToolDef[];
	
	    static createFrom(source: any = {}) {
	        return new MCPConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = this.convertValues(source["server"], null);
	        this.tools = this.convertValues(source["tools"], MCPToolDef);
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
	export class DependenciesConfig {
	    npm?: string[];
	    pip?: string[];
	    commands?: string[];
	    skills?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DependenciesConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.npm = source["npm"];
	        this.pip = source["pip"];
	        this.commands = source["commands"];
	        this.skills = source["skills"];
	    }
	}
	export class TriggerFilter {
	    tools?: string[];
	    files?: string[];
	
	    static createFrom(source: any = {}) {
	        return new TriggerFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tools = source["tools"];
	        this.files = source["files"];
	    }
	}
	export class TriggerConfig {
	    events?: string[];
	    // Go type: TriggerFilter
	    filters?: any;
	    priority?: number;
	
	    static createFrom(source: any = {}) {
	        return new TriggerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = source["events"];
	        this.filters = this.convertValues(source["filters"], null);
	        this.priority = source["priority"];
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
	export class ModelConfig {
	    preferred?: string;
	    fallback?: string;
	    temperature?: number;
	    maxTokens?: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preferred = source["preferred"];
	        this.fallback = source["fallback"];
	        this.temperature = source["temperature"];
	        this.maxTokens = source["maxTokens"];
	    }
	}
	export class InteractiveConf {
	    confirmDestructive?: boolean;
	    showProgress?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InteractiveConf(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.confirmDestructive = source["confirmDestructive"];
	        this.showProgress = source["showProgress"];
	    }
	}
	export class CacheConfig {
	    enabled?: boolean;
	    ttlSeconds?: number;
	    keyFields?: string[];
	
	    static createFrom(source: any = {}) {
	        return new CacheConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.ttlSeconds = source["ttlSeconds"];
	        this.keyFields = source["keyFields"];
	    }
	}
	export class RetryConfig {
	    maxAttempts?: number;
	    backoffMs?: number;
	
	    static createFrom(source: any = {}) {
	        return new RetryConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxAttempts = source["maxAttempts"];
	        this.backoffMs = source["backoffMs"];
	    }
	}
	export class BehaviorConfig {
	    timeout?: number;
	    // Go type: RetryConfig
	    retry?: any;
	    // Go type: CacheConfig
	    cache?: any;
	    // Go type: InteractiveConf
	    interactive?: any;
	    // Go type: ModelConfig
	    model?: any;
	
	    static createFrom(source: any = {}) {
	        return new BehaviorConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeout = source["timeout"];
	        this.retry = this.convertValues(source["retry"], null);
	        this.cache = this.convertValues(source["cache"], null);
	        this.interactive = this.convertValues(source["interactive"], null);
	        this.model = this.convertValues(source["model"], null);
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
	export class OutputConfig {
	    format?: string;
	    schema?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new OutputConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.schema = source["schema"];
	    }
	}
	export class ContextReq {
	    requiresProject?: boolean;
	    requiresGit?: boolean;
	    requiresPackageJson?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContextReq(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requiresProject = source["requiresProject"];
	        this.requiresGit = source["requiresGit"];
	        this.requiresPackageJson = source["requiresPackageJson"];
	    }
	}
	export class ArgumentDef {
	    name: string;
	    type: string;
	    description?: string;
	    required?: boolean;
	    default?: any;
	    enum?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ArgumentDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.required = source["required"];
	        this.default = source["default"];
	        this.enum = source["enum"];
	    }
	}
	export class InputConfig {
	    arguments?: ArgumentDef[];
	    // Go type: ContextReq
	    context?: any;
	
	    static createFrom(source: any = {}) {
	        return new InputConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.arguments = this.convertValues(source["arguments"], ArgumentDef);
	        this.context = this.convertValues(source["context"], null);
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
	export class BashCommands {
	    allowed?: string[];
	    denied?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BashCommands(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowed = source["allowed"];
	        this.denied = source["denied"];
	    }
	}
	export class ToolPermissions {
	    allowed?: string[];
	    denied?: string[];
	    // Go type: BashCommands
	    bashCommands?: any;
	
	    static createFrom(source: any = {}) {
	        return new ToolPermissions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowed = source["allowed"];
	        this.denied = source["denied"];
	        this.bashCommands = this.convertValues(source["bashCommands"], null);
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
	export class NetworkPermissions {
	    allowedHosts?: string[];
	    deniedHosts?: string[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkPermissions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowedHosts = source["allowedHosts"];
	        this.deniedHosts = source["deniedHosts"];
	    }
	}
	export class FilesystemPermissions {
	    read?: string[];
	    write?: string[];
	    deny?: string[];
	
	    static createFrom(source: any = {}) {
	        return new FilesystemPermissions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.read = source["read"];
	        this.write = source["write"];
	        this.deny = source["deny"];
	    }
	}
	export class Skill {
	    name: string;
	    version: string;
	    description: string;
	    displayName?: string;
	    author?: string;
	    authorEmail?: string;
	    authorUrl?: string;
	    license?: string;
	    repository?: string;
	    homepage?: string;
	    keywords?: string[];
	    category?: string;
	    subcategory?: string;
	    type?: string;
	    difficulty?: string;
	    audience?: string[];
	    minVersion?: string;
	    maxVersion?: string;
	    platforms?: string[];
	    languages?: string[];
	    frameworks?: string[];
	    disableModelInvocation?: boolean;
	    userInvocable?: boolean;
	    argumentHint?: string;
	    context?: string;
	    agent?: string;
	    model?: string;
	    // Go type: FilesystemPermissions
	    filesystem?: any;
	    // Go type: NetworkPermissions
	    network?: any;
	    // Go type: ToolPermissions
	    tools?: any;
	    // Go type: InputConfig
	    input?: any;
	    // Go type: OutputConfig
	    output?: any;
	    // Go type: BehaviorConfig
	    behavior?: any;
	    // Go type: TriggerConfig
	    triggers?: any;
	    hooks?: any;
	    // Go type: DependenciesConfig
	    dependencies?: any;
	    // Go type: MCPConfig
	    mcp?: any;
	    slug: string;
	    source: string;
	    content: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.displayName = source["displayName"];
	        this.author = source["author"];
	        this.authorEmail = source["authorEmail"];
	        this.authorUrl = source["authorUrl"];
	        this.license = source["license"];
	        this.repository = source["repository"];
	        this.homepage = source["homepage"];
	        this.keywords = source["keywords"];
	        this.category = source["category"];
	        this.subcategory = source["subcategory"];
	        this.type = source["type"];
	        this.difficulty = source["difficulty"];
	        this.audience = source["audience"];
	        this.minVersion = source["minVersion"];
	        this.maxVersion = source["maxVersion"];
	        this.platforms = source["platforms"];
	        this.languages = source["languages"];
	        this.frameworks = source["frameworks"];
	        this.disableModelInvocation = source["disableModelInvocation"];
	        this.userInvocable = source["userInvocable"];
	        this.argumentHint = source["argumentHint"];
	        this.context = source["context"];
	        this.agent = source["agent"];
	        this.model = source["model"];
	        this.filesystem = this.convertValues(source["filesystem"], null);
	        this.network = this.convertValues(source["network"], null);
	        this.tools = this.convertValues(source["tools"], null);
	        this.input = this.convertValues(source["input"], null);
	        this.output = this.convertValues(source["output"], null);
	        this.behavior = this.convertValues(source["behavior"], null);
	        this.triggers = this.convertValues(source["triggers"], null);
	        this.hooks = source["hooks"];
	        this.dependencies = this.convertValues(source["dependencies"], null);
	        this.mcp = this.convertValues(source["mcp"], null);
	        this.slug = source["slug"];
	        this.source = source["source"];
	        this.content = source["content"];
	        this.path = source["path"];
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
	export class SkillInfo {
	    name: string;
	    version: string;
	    description: string;
	    displayName?: string;
	    author?: string;
	    authorEmail?: string;
	    authorUrl?: string;
	    license?: string;
	    repository?: string;
	    homepage?: string;
	    keywords?: string[];
	    category?: string;
	    subcategory?: string;
	    type?: string;
	    difficulty?: string;
	    audience?: string[];
	    minVersion?: string;
	    maxVersion?: string;
	    platforms?: string[];
	    languages?: string[];
	    frameworks?: string[];
	    disableModelInvocation?: boolean;
	    userInvocable?: boolean;
	    argumentHint?: string;
	    context?: string;
	    agent?: string;
	    model?: string;
	    // Go type: FilesystemPermissions
	    filesystem?: any;
	    // Go type: NetworkPermissions
	    network?: any;
	    // Go type: ToolPermissions
	    tools?: any;
	    // Go type: InputConfig
	    input?: any;
	    // Go type: OutputConfig
	    output?: any;
	    // Go type: BehaviorConfig
	    behavior?: any;
	    // Go type: TriggerConfig
	    triggers?: any;
	    hooks?: any;
	    // Go type: DependenciesConfig
	    dependencies?: any;
	    // Go type: MCPConfig
	    mcp?: any;
	    slug: string;
	    source: string;
	    autoLoad?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SkillInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.displayName = source["displayName"];
	        this.author = source["author"];
	        this.authorEmail = source["authorEmail"];
	        this.authorUrl = source["authorUrl"];
	        this.license = source["license"];
	        this.repository = source["repository"];
	        this.homepage = source["homepage"];
	        this.keywords = source["keywords"];
	        this.category = source["category"];
	        this.subcategory = source["subcategory"];
	        this.type = source["type"];
	        this.difficulty = source["difficulty"];
	        this.audience = source["audience"];
	        this.minVersion = source["minVersion"];
	        this.maxVersion = source["maxVersion"];
	        this.platforms = source["platforms"];
	        this.languages = source["languages"];
	        this.frameworks = source["frameworks"];
	        this.disableModelInvocation = source["disableModelInvocation"];
	        this.userInvocable = source["userInvocable"];
	        this.argumentHint = source["argumentHint"];
	        this.context = source["context"];
	        this.agent = source["agent"];
	        this.model = source["model"];
	        this.filesystem = this.convertValues(source["filesystem"], null);
	        this.network = this.convertValues(source["network"], null);
	        this.tools = this.convertValues(source["tools"], null);
	        this.input = this.convertValues(source["input"], null);
	        this.output = this.convertValues(source["output"], null);
	        this.behavior = this.convertValues(source["behavior"], null);
	        this.triggers = this.convertValues(source["triggers"], null);
	        this.hooks = source["hooks"];
	        this.dependencies = this.convertValues(source["dependencies"], null);
	        this.mcp = this.convertValues(source["mcp"], null);
	        this.slug = source["slug"];
	        this.source = source["source"];
	        this.autoLoad = source["autoLoad"];
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

export namespace speech {
	
	export class AudioResult {
	    audio: string;
	    mimeType: string;
	    cached: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AudioResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audio = source["audio"];
	        this.mimeType = source["mimeType"];
	        this.cached = source["cached"];
	    }
	}
	export class SpeechModelInfo {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class TTSModelInfo {
	    id: string;
	    name: string;
	    description?: string;
	    provider: string;
	    selection_mode: string;
	
	    static createFrom(source: any = {}) {
	        return new TTSModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.provider = source["provider"];
	        this.selection_mode = source["selection_mode"];
	    }
	}
	export class TTSVoiceInfo {
	    id: string;
	    name: string;
	    description: string;
	    gender: string;
	    provider: string;
	    model_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new TTSVoiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.gender = source["gender"];
	        this.provider = source["provider"];
	        this.model_id = source["model_id"];
	    }
	}
	export class TranscriptionResult {
	    text: string;
	    language?: string;
	    duration?: number;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new TranscriptionResult(source);
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

export namespace terminal {
	
	export class HistoryEntry {
	    id: string;
	    command: string;
	    output: string;
	    exitCode: number;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    endedAt: any;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.command = source["command"];
	        this.output = source["output"];
	        this.exitCode = source["exitCode"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.endedAt = this.convertValues(source["endedAt"], null);
	        this.source = source["source"];
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
	export class ManagerStats {
	    totalSessions: number;
	    idleSessions: number;
	    busySessions: number;
	    maxSessions: number;
	
	    static createFrom(source: any = {}) {
	        return new ManagerStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalSessions = source["totalSessions"];
	        this.idleSessions = source["idleSessions"];
	        this.busySessions = source["busySessions"];
	        this.maxSessions = source["maxSessions"];
	    }
	}
	export class SessionInfo {
	    id: string;
	    name: string;
	    cwd: string;
	    state: string;
	    shell: string;
	    createdAt: string;
	    lastUsed: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cwd = source["cwd"];
	        this.state = source["state"];
	        this.shell = source["shell"];
	        this.createdAt = source["createdAt"];
	        this.lastUsed = source["lastUsed"];
	    }
	}

}

export namespace updater {
	
	export class UpdateInfo {
	    available: boolean;
	    currentVersion: string;
	    latestVersion: string;
	    releaseNotes?: string;
	    releaseDate?: string;
	    downloadSize?: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.releaseNotes = source["releaseNotes"];
	        this.releaseDate = source["releaseDate"];
	        this.downloadSize = source["downloadSize"];
	    }
	}

}

export namespace workspace {
	
	export class Tab {
	    id: string;
	    type: string;
	    conversation_id?: string;
	    title: string;
	    position: number;
	    profile_override?: Record<string, any>;
	    state?: Record<string, any>;
	    content_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new Tab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.conversation_id = source["conversation_id"];
	        this.title = source["title"];
	        this.position = source["position"];
	        this.profile_override = source["profile_override"];
	        this.state = source["state"];
	        this.content_id = source["content_id"];
	    }
	}
	export class TabsState {
	    active: string;
	    items: Tab[];
	
	    static createFrom(source: any = {}) {
	        return new TabsState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.items = this.convertValues(source["items"], Tab);
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
	export class Workspace {
	    id: string;
	    name: string;
	    profile?: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    last_used: any;
	    tabs: TabsState;
	
	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.profile = source["profile"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.last_used = this.convertValues(source["last_used"], null);
	        this.tabs = this.convertValues(source["tabs"], TabsState);
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
	export class WorkspaceInfo {
	    id: string;
	    name: string;
	    path: string;
	    profile: string;
	    tab_count: number;
	    is_active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.profile = source["profile"];
	        this.tab_count = source["tab_count"];
	        this.is_active = source["is_active"];
	    }
	}

}

