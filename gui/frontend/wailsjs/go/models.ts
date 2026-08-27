export namespace bridge {

	export class BuildInfo {
	    version: string;
	    go_version: string;
	    commit?: string;
	    build_date?: string;

	    static createFrom(source: any = {}) {
	        return new BuildInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.go_version = source["go_version"];
	        this.commit = source["commit"];
	        this.build_date = source["build_date"];
	    }
	}
	export class AssetSnapshot {
	    engines: gdit.InstalledVersion[];
	    sdks: gdit.SDKInfo[];
	    sources: gdit.SourceInfo[];
	    templates: gdit.TemplateInfo[];
	    orphans: gdit.OrphanAsset[];

	    static createFrom(source: any = {}) {
	        return new AssetSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engines = this.convertValues(source["engines"], gdit.InstalledVersion);
	        this.sdks = this.convertValues(source["sdks"], gdit.SDKInfo);
	        this.sources = this.convertValues(source["sources"], gdit.SourceInfo);
	        this.templates = this.convertValues(source["templates"], gdit.TemplateInfo);
	        this.orphans = this.convertValues(source["orphans"], gdit.OrphanAsset);
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
	export class AppSnapshot {
	    root: string;
	    instances: gdit.InstanceInfo[];
	    current?: gdit.InstanceInfo;
	    assets: AssetSnapshot;
	    doctor: gdit.DoctorReport;
	    gui: gdit.GUISettings;
	    build: BuildInfo;
	    issues?: string[];

	    static createFrom(source: any = {}) {
	        return new AppSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.instances = this.convertValues(source["instances"], gdit.InstanceInfo);
	        this.current = this.convertValues(source["current"], gdit.InstanceInfo);
	        this.assets = this.convertValues(source["assets"], AssetSnapshot);
	        this.doctor = this.convertValues(source["doctor"], gdit.DoctorReport);
	        this.gui = this.convertValues(source["gui"], gdit.GUISettings);
	        this.build = this.convertValues(source["build"], BuildInfo);
	        this.issues = source["issues"];
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


	export class CandidateWarning {
	    source?: string;
	    message: string;

	    static createFrom(source: any = {}) {
	        return new CandidateWarning(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.message = source["message"];
	    }
	}
	export class EffectiveEnvVar {
	    key: string;
	    value: string;
	    origin: string;
	    sensitive: boolean;

	    static createFrom(source: any = {}) {
	        return new EffectiveEnvVar(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.origin = source["origin"];
	        this.sensitive = source["sensitive"];
	    }
	}
	export class EffectiveEnvView {
	    vars: EffectiveEnvVar[];
	    args: string[];

	    static createFrom(source: any = {}) {
	        return new EffectiveEnvView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vars = this.convertValues(source["vars"], EffectiveEnvVar);
	        this.args = source["args"];
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
	export class EngineCandidateResult {
	    channels: gdit.EngineChannel[];
	    warnings: CandidateWarning[];

	    static createFrom(source: any = {}) {
	        return new EngineCandidateResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channels = this.convertValues(source["channels"], gdit.EngineChannel);
	        this.warnings = this.convertValues(source["warnings"], CandidateWarning);
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
	export class EnvironmentDetails {
	    configured: gdit.ConfiguredEnvView;
	    effective: EffectiveEnvView;
	    effective_error?: string;

	    static createFrom(source: any = {}) {
	        return new EnvironmentDetails(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = this.convertValues(source["configured"], gdit.ConfiguredEnvView);
	        this.effective = this.convertValues(source["effective"], EffectiveEnvView);
	        this.effective_error = source["effective_error"];
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
	export class InstanceDetails {
	    instance: gdit.InstanceInfo;
	    env: gdit.EnvView;
	    configured_env: gdit.ConfiguredEnvView;
	    env_error?: string;
	    templates: gdit.TemplateInfo[];

	    static createFrom(source: any = {}) {
	        return new InstanceDetails(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instance = this.convertValues(source["instance"], gdit.InstanceInfo);
	        this.env = this.convertValues(source["env"], gdit.EnvView);
	        this.configured_env = this.convertValues(source["configured_env"], gdit.ConfiguredEnvView);
	        this.env_error = source["env_error"];
	        this.templates = this.convertValues(source["templates"], gdit.TemplateInfo);
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
	export class OperationStart {
	    operation_id: string;

	    static createFrom(source: any = {}) {
	        return new OperationStart(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operation_id = source["operation_id"];
	    }
	}
	export class SDKCandidateResult {
	    channels: gdit.SDKChannel[];
	    warnings: CandidateWarning[];

	    static createFrom(source: any = {}) {
	        return new SDKCandidateResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channels = this.convertValues(source["channels"], gdit.SDKChannel);
	        this.warnings = this.convertValues(source["warnings"], CandidateWarning);
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
	export class SessionSnapshot {
	    sessions: gdit.SessionInfo[];

	    static createFrom(source: any = {}) {
	        return new SessionSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], gdit.SessionInfo);
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

export namespace gdit {

	export class AvailableVersion {
	    version: string;
	    editions: string[];
	    sources: string[];

	    static createFrom(source: any = {}) {
	        return new AvailableVersion(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.editions = source["editions"];
	        this.sources = source["sources"];
	    }
	}
	export class CheckResult {
	    code: string;
	    status: string;
	    message: string;
	    suggest?: string;
	    details?: string[];

	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.suggest = source["suggest"];
	        this.details = source["details"];
	    }
	}
	export class ConfiguredEnvVar {
	    key: string;
	    value: string;
	    scope: string;
	    editable: boolean;
	    sensitive: boolean;

	    static createFrom(source: any = {}) {
	        return new ConfiguredEnvVar(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.scope = source["scope"];
	        this.editable = source["editable"];
	        this.sensitive = source["sensitive"];
	    }
	}
	export class ConfiguredEnvView {
	    vars: ConfiguredEnvVar[];

	    static createFrom(source: any = {}) {
	        return new ConfiguredEnvView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vars = this.convertValues(source["vars"], ConfiguredEnvVar);
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
	export class DoctorReport {
	    root: string;
	    items: CheckResult[];
	    ok_count: number;
	    warn_count: number;
	    error_count: number;

	    static createFrom(source: any = {}) {
	        return new DoctorReport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.items = this.convertValues(source["items"], CheckResult);
	        this.ok_count = source["ok_count"];
	        this.warn_count = source["warn_count"];
	        this.error_count = source["error_count"];
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
	export class EngineChannel {
	    name: string;
	    versions: AvailableVersion[];

	    static createFrom(source: any = {}) {
	        return new EngineChannel(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.versions = this.convertValues(source["versions"], AvailableVersion);
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
	export class EnvVar {
	    key: string;
	    value: string;
	    origin: string;

	    static createFrom(source: any = {}) {
	        return new EnvVar(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.origin = source["origin"];
	    }
	}
	export class EnvView {
	    vars: EnvVar[];
	    args: string[];

	    static createFrom(source: any = {}) {
	        return new EnvView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vars = this.convertValues(source["vars"], EnvVar);
	        this.args = source["args"];
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
	export class GUISettings {
	    titlebar_style: string;

	    static createFrom(source: any = {}) {
	        return new GUISettings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.titlebar_style = source["titlebar_style"];
	    }
	}
	export class InstallEntryRequest {
	    name: string;
	    version: string;
	    edition: string;
	    source?: string;
	    sdk_strategy?: string;
	    sdk_version?: string;
	    set_current?: boolean;
	    template?: boolean;

	    static createFrom(source: any = {}) {
	        return new InstallEntryRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.edition = source["edition"];
	        this.source = source["source"];
	        this.sdk_strategy = source["sdk_strategy"];
	        this.sdk_version = source["sdk_version"];
	        this.set_current = source["set_current"];
	        this.template = source["template"];
	    }
	}
	export class InstallSuggestionRequest {
	    project_dir: string;
	    name: string;
	    sdk_strategy?: string;
	    sdk_version?: string;
	    set_current?: boolean;
	    include_template?: boolean;

	    static createFrom(source: any = {}) {
	        return new InstallSuggestionRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_dir = source["project_dir"];
	        this.name = source["name"];
	        this.sdk_strategy = source["sdk_strategy"];
	        this.sdk_version = source["sdk_version"];
	        this.set_current = source["set_current"];
	        this.include_template = source["include_template"];
	    }
	}
	export class Target {
	    os: string;
	    arch: string;

	    static createFrom(source: any = {}) {
	        return new Target(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.arch = source["arch"];
	    }
	}
	export class InstalledVersion {
	    id: string;
	    version: string;
	    edition: string;
	    target: Target;
	    source: string;
	    checksum_algorithm: string;
	    checksum: string;
	    launcher: string;
	    installed_at: string;

	    static createFrom(source: any = {}) {
	        return new InstalledVersion(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.edition = source["edition"];
	        this.target = this.convertValues(source["target"], Target);
	        this.source = source["source"];
	        this.checksum_algorithm = source["checksum_algorithm"];
	        this.checksum = source["checksum"];
	        this.launcher = source["launcher"];
	        this.installed_at = source["installed_at"];
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
	export class InstanceInfo {
	    id: string;
	    name: string;
	    engine: string;
	    edition: string;
	    sdk_strategy: string;
	    sdk: string;
	    current: boolean;
	    template: string;
	    template_missing: boolean;
	    icon: string;
	    resolved_icon: string;
	    icon_missing: boolean;
	    icon_background: string;

	    static createFrom(source: any = {}) {
	        return new InstanceInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.engine = source["engine"];
	        this.edition = source["edition"];
	        this.sdk_strategy = source["sdk_strategy"];
	        this.sdk = source["sdk"];
	        this.current = source["current"];
	        this.template = source["template"];
	        this.template_missing = source["template_missing"];
	        this.icon = source["icon"];
	        this.resolved_icon = source["resolved_icon"];
	        this.icon_missing = source["icon_missing"];
	        this.icon_background = source["icon_background"];
	    }
	}
	export class OrphanAsset {
	    kind: string;
	    id: string;
	    size: number;
	    path: string;

	    static createFrom(source: any = {}) {
	        return new OrphanAsset(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.size = source["size"];
	        this.path = source["path"];
	    }
	}
	export class SDKChannel {
	    major_minor: string;
	    phase: string;
	    release_type: string;
	    versions: string[];

	    static createFrom(source: any = {}) {
	        return new SDKChannel(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.major_minor = source["major_minor"];
	        this.phase = source["phase"];
	        this.release_type = source["release_type"];
	        this.versions = source["versions"];
	    }
	}
	export class SDKInfo {
	    version: string;
	    kind: string;
	    path: string;

	    static createFrom(source: any = {}) {
	        return new SDKInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.kind = source["kind"];
	        this.path = source["path"];
	    }
	}
	export class SessionInfo {
	    session_id: string;
	    instance_id: string;
	    instance_name: string;
	    engine_id: string;
	    pid: number;
	    // Go type: time
	    started_at: any;
	    status: string;

	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.instance_id = source["instance_id"];
	        this.instance_name = source["instance_name"];
	        this.engine_id = source["engine_id"];
	        this.pid = source["pid"];
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.status = source["status"];
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
	export class SetInstanceIconRequest {
	    icon: string;
	    source_path?: string;
	    background?: string;

	    static createFrom(source: any = {}) {
	        return new SetInstanceIconRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.icon = source["icon"];
	        this.source_path = source["source_path"];
	        this.background = source["background"];
	    }
	}
	export class SourceInfo {
	    name: string;
	    kind: string;
	    disabled: boolean;

	    static createFrom(source: any = {}) {
	        return new SourceInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.disabled = source["disabled"];
	    }
	}

	export class TemplateInfo {
	    id: string;
	    version: string;
	    edition: string;
	    source: string;
	    checksum_algorithm: string;
	    checksum: string;
	    archive_name: string;
	    path: string;
	    size: number;
	    installed_at: string;
	    references: string[];

	    static createFrom(source: any = {}) {
	        return new TemplateInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.edition = source["edition"];
	        this.source = source["source"];
	        this.checksum_algorithm = source["checksum_algorithm"];
	        this.checksum = source["checksum"];
	        this.archive_name = source["archive_name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.installed_at = source["installed_at"];
	        this.references = source["references"];
	    }
	}

}

