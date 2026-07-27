export namespace a2ui {

	export class ClientAction {
	    name: string;
	    surfaceId: string;
	    sourceComponentId: string;
	    timestamp: string;
	    context: Record<string, any>;

	    static createFrom(source: any = {}) {
	        return new ClientAction(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.surfaceId = source["surfaceId"];
	        this.sourceComponentId = source["sourceComponentId"];
	        this.timestamp = source["timestamp"];
	        this.context = source["context"];
	    }
	}

}

export namespace agentcore {

	export class CacheControlMarker {
	    type: string;
	    ttl?: string;

	    static createFrom(source: any = {}) {
	        return new CacheControlMarker(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.ttl = source["ttl"];
	    }
	}
	export class ThinkingConfig {
	    include_thoughts?: boolean;
	    display?: string;
	    effort?: string;
	    budget?: number;

	    static createFrom(source: any = {}) {
	        return new ThinkingConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.include_thoughts = source["include_thoughts"];
	        this.display = source["display"];
	        this.effort = source["effort"];
	        this.budget = source["budget"];
	    }
	}
	export class ResponseFormatJSONSchemaConfig {
	    name: string;
	    description?: string;
	    schema: Record<string, any>;
	    strict?: boolean;

	    static createFrom(source: any = {}) {
	        return new ResponseFormatJSONSchemaConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.schema = source["schema"];
	        this.strict = source["strict"];
	    }
	}
	export class ResponseFormat {
	    type: string;
	    json_schema?: ResponseFormatJSONSchemaConfig;

	    static createFrom(source: any = {}) {
	        return new ResponseFormat(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.json_schema = this.convertValues(source["json_schema"], ResponseFormatJSONSchemaConfig);
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
	export class CallConfig {
	    model?: string;
	    response_format?: ResponseFormat;
	    thinking?: ThinkingConfig;
	    skills?: string[];

	    static createFrom(source: any = {}) {
	        return new CallConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.response_format = this.convertValues(source["response_format"], ResponseFormat);
	        this.thinking = this.convertValues(source["thinking"], ThinkingConfig);
	        this.skills = source["skills"];
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
	export class ContentBlock {
	    kind: string;
	    text?: string;
	    url?: string;
	    media_type?: string;
	    detail?: string;
	    signature?: string;
	    tool_call_id?: string;
	    name?: string;
	    arguments?: string;

	    static createFrom(source: any = {}) {
	        return new ContentBlock(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.url = source["url"];
	        this.media_type = source["media_type"];
	        this.detail = source["detail"];
	        this.signature = source["signature"];
	        this.tool_call_id = source["tool_call_id"];
	        this.name = source["name"];
	        this.arguments = source["arguments"];
	    }
	}
	export class ToolCall {
	    id: string;
	    name: string;
	    arguments: string;

	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.arguments = source["arguments"];
	    }
	}
	export class Message {
	    id?: string;
	    role: string;
	    content?: string;
	    tool_calls?: ToolCall[];
	    tool_call_id?: string;
	    name?: string;
	    type?: string;
	    metadata?: Record<string, any>;
	    cache_control?: CacheControlMarker;
	    blocks?: ContentBlock[];
	    invocation_id?: string;

	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
	        this.tool_call_id = source["tool_call_id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.metadata = source["metadata"];
	        this.cache_control = this.convertValues(source["cache_control"], CacheControlMarker);
	        this.blocks = this.convertValues(source["blocks"], ContentBlock);
	        this.invocation_id = source["invocation_id"];
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



	export class TokenUsage {
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;

	    static createFrom(source: any = {}) {
	        return new TokenUsage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	    }
	}

}

export namespace main {

	export class FileEntry {
	    name: string;
	    isDir: boolean;
	    size: number;
	    modTime: number;

	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	    }
	}
	export class ThreadSummary {
	    key: string;
	    title: string;
	    // Go type: time
	    updatedAt: any;
	    messageN: number;

	    static createFrom(source: any = {}) {
	        return new ThreadSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.title = source["title"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.messageN = source["messageN"];
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

export namespace server {

	export class ChatRequest {
	    message: string;
	    stream: boolean;
	    thread_id?: string;
	    model?: string;
	    response_format?: agentcore.ResponseFormat;
	    thinking?: agentcore.ThinkingConfig;
	    skills?: string[];

	    static createFrom(source: any = {}) {
	        return new ChatRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.stream = source["stream"];
	        this.thread_id = source["thread_id"];
	        this.model = source["model"];
	        this.response_format = this.convertValues(source["response_format"], agentcore.ResponseFormat);
	        this.thinking = this.convertValues(source["thinking"], agentcore.ThinkingConfig);
	        this.skills = source["skills"];
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
	export class HealthInfo {
	    provider: string;
	    model: string;
	    version: string;
	    uptime: string;

	    static createFrom(source: any = {}) {
	        return new HealthInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.version = source["version"];
	        this.uptime = source["uptime"];
	    }
	}

}

export namespace session {

	export class Info {
	    id: string;
	    name?: string;
	    label?: string;
	    summary?: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    parent_session?: string;
	    cwd?: string;
	    message_count: number;
	    version: number;

	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.label = source["label"];
	        this.summary = source["summary"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.parent_session = source["parent_session"];
	        this.cwd = source["cwd"];
	        this.message_count = source["message_count"];
	        this.version = source["version"];
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
	export class ThreadMessage {
	    entry_id?: string;
	    message: agentcore.Message;

	    static createFrom(source: any = {}) {
	        return new ThreadMessage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entry_id = source["entry_id"];
	        this.message = this.convertValues(source["message"], agentcore.Message);
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
	export class ThreadSnapshot {
	    info: Info;
	    messages: agentcore.Message[];
	    transcript?: ThreadMessage[];
	    status: string;
	    turn: number;
	    total_usage: agentcore.TokenUsage;
	    config?: agentcore.CallConfig;
	    thinking?: agentcore.ThinkingConfig;

	    static createFrom(source: any = {}) {
	        return new ThreadSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.info = this.convertValues(source["info"], Info);
	        this.messages = this.convertValues(source["messages"], agentcore.Message);
	        this.transcript = this.convertValues(source["transcript"], ThreadMessage);
	        this.status = source["status"];
	        this.turn = source["turn"];
	        this.total_usage = this.convertValues(source["total_usage"], agentcore.TokenUsage);
	        this.config = this.convertValues(source["config"], agentcore.CallConfig);
	        this.thinking = this.convertValues(source["thinking"], agentcore.ThinkingConfig);
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
