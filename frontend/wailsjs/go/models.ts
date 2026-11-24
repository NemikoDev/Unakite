export namespace session {
	
	export class FileTransfer {
	    id: string;
	    fileName: string;
	    fileSize: number;
	    direction: string;
	    progress: number;
	    status: string;
	    error?: string;
	    path?: string;
	    startedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new FileTransfer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.fileName = source["fileName"];
	        this.fileSize = source["fileSize"];
	        this.direction = source["direction"];
	        this.progress = source["progress"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.path = source["path"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class PeerInfo {
	    id: string;
	    nickname: string;
	
	    static createFrom(source: any = {}) {
	        return new PeerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.nickname = source["nickname"];
	    }
	}
	export class Message {
	    id: string;
	    sender: PeerInfo;
	    content: string;
	    timestamp: number;
	    isOwn: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sender = this.convertValues(source["sender"], PeerInfo);
	        this.content = source["content"];
	        this.timestamp = source["timestamp"];
	        this.isOwn = source["isOwn"];
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
	
	export class State {
	    inRoom: boolean;
	    code: string;
	    self: PeerInfo;
	    peer?: PeerInfo;
	    closesAt: number;
	    transfers: FileTransfer[];
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inRoom = source["inRoom"];
	        this.code = source["code"];
	        this.self = this.convertValues(source["self"], PeerInfo);
	        this.peer = this.convertValues(source["peer"], PeerInfo);
	        this.closesAt = source["closesAt"];
	        this.transfers = this.convertValues(source["transfers"], FileTransfer);
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

