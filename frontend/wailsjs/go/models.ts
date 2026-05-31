export namespace main {
	
	export class P2PMessage {
	    type: string;
	    timestamp: number;
	    data?: any;
	
	    static createFrom(source: any = {}) {
	        return new P2PMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.timestamp = source["timestamp"];
	        this.data = source["data"];
	    }
	}
	export class PeerInfo {
	    id: string;
	    name: string;
	    isHost: boolean;
	    isLeader: boolean;
	    connected: boolean;
	    // Go type: time
	    lastSeen: any;
	
	    static createFrom(source: any = {}) {
	        return new PeerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.isHost = source["isHost"];
	        this.isLeader = source["isLeader"];
	        this.connected = source["connected"];
	        this.lastSeen = this.convertValues(source["lastSeen"], null);
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
	export class PlaybackState {
	    isPlaying: boolean;
	    position: number;
	    duration: number;
	    speed: number;
	    timestamp: number;
	    playbackRate: number;
	
	    static createFrom(source: any = {}) {
	        return new PlaybackState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isPlaying = source["isPlaying"];
	        this.position = source["position"];
	        this.duration = source["duration"];
	        this.speed = source["speed"];
	        this.timestamp = source["timestamp"];
	        this.playbackRate = source["playbackRate"];
	    }
	}
	export class SyncCommand {
	    type: string;
	    timestamp: number;
	    data?: any;
	
	    static createFrom(source: any = {}) {
	        return new SyncCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.timestamp = source["timestamp"];
	        this.data = source["data"];
	    }
	}
	export class SyncStats {
	    latency: number;
	    drift: number;
	    syncAccuracy: number;
	    rebufferingCount: number;
	    rtt: number;
	    lastSyncTime: number;
	    syncTolerance: number;
	    correctionCount: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.latency = source["latency"];
	        this.drift = source["drift"];
	        this.syncAccuracy = source["syncAccuracy"];
	        this.rebufferingCount = source["rebufferingCount"];
	        this.rtt = source["rtt"];
	        this.lastSyncTime = source["lastSyncTime"];
	        this.syncTolerance = source["syncTolerance"];
	        this.correctionCount = source["correctionCount"];
	    }
	}
	export class TorrentFile {
	    name: string;
	    path: string;
	    size: number;
	    offset: number;
	    progress: number;
	
	    static createFrom(source: any = {}) {
	        return new TorrentFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.offset = source["offset"];
	        this.progress = source["progress"];
	    }
	}
	export class TorrentInfo {
	    hash: string;
	    name: string;
	    size: number;
	    progress: number;
	    peers: number;
	    seeds: number;
	    downloadSpeed: number;
	    uploadSpeed: number;
	    status: string;
	    files: TorrentFile[];
	    speed: number;
	
	    static createFrom(source: any = {}) {
	        return new TorrentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.progress = source["progress"];
	        this.peers = source["peers"];
	        this.seeds = source["seeds"];
	        this.downloadSpeed = source["downloadSpeed"];
	        this.uploadSpeed = source["uploadSpeed"];
	        this.status = source["status"];
	        this.files = this.convertValues(source["files"], TorrentFile);
	        this.speed = source["speed"];
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

export namespace webrtc {
	
	export class ICECandidateInit {
	    candidate: string;
	    sdpMid?: string;
	    sdpMLineIndex?: number;
	    usernameFragment?: string;
	
	    static createFrom(source: any = {}) {
	        return new ICECandidateInit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.candidate = source["candidate"];
	        this.sdpMid = source["sdpMid"];
	        this.sdpMLineIndex = source["sdpMLineIndex"];
	        this.usernameFragment = source["usernameFragment"];
	    }
	}

}

