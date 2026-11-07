import Foundation

/// A remote preview is never a download/cache shortcut. Each request presents
/// the current viewer's proof; one refresh is allowed, for this same actor only.
@available(iOS 13.0, *)
final class PMRemoteAsset {
    private weak var authority:PMVaultAuthority?
    private let actor:PMVaultActor,source:String,authorize:(PMVaultActor,String,String)throws->[String:Any]
    private let lock=NSRecursiveLock()
    private var url:URL!,proof="",etag="",refreshed=false
    private(set) var size:Int64=0
    private(set) var contentType="application/octet-stream"
    init(authority:PMVaultAuthority,actor:PMVaultActor,source:String,authorize:@escaping(PMVaultActor,String,String)throws->[String:Any])throws {
        self.authority=authority;self.actor=actor;self.source=source;self.authorize=authorize
        try refresh();_ = try request(offset:nil,count:0)
    }
    private func refresh()throws {
        guard authority?.canPreview(actor)==true else{throw PMProtectedError.rejected}
        let grant=try authorize(actor,source,"preview")
        guard let address=grant["url"] as? String,let value=grant["resourceProof"] as? String,value.range(of:"^[0-9a-f]{64}$",options:.regularExpression) != nil,
          let parsed=URL(string:address),let base=URL(string:actor.origin),let parts=URLComponents(url:parsed,resolvingAgainstBaseURL:false),parts.user==nil,parts.password==nil,parts.fragment==nil,
          parsed.scheme=="https",parsed.host==base.host,parsed.port==base.port,parsed.path.hasPrefix("/attachments/"),authority?.canPreview(actor)==true else{throw PMProtectedError.rejected}
        url=parsed;proof=value
    }
    private func request(offset:Int64?,count:Int)throws->Data {
        guard let authority=authority,authority.canPreview(actor) else{throw PMProtectedError.rejected}
        var request=URLRequest(url:url,cachePolicy:.reloadIgnoringLocalCacheData,timeoutInterval:8);request.httpMethod=offset==nil ? "HEAD":"GET";request.setValue(proof,forHTTPHeaderField:"X-Puty-Resource-Proof");request.setValue("no-store",forHTTPHeaderField:"Cache-Control")
        if let offset=offset{request.setValue("bytes=\(offset)-\(offset+Int64(count)-1)",forHTTPHeaderField:"Range");if !etag.isEmpty{request.setValue(etag,forHTTPHeaderField:"If-Range")}}
        var body=Data(),denied=false
        let transport=PMProtectedHTTP(timeout:8,maximum:offset==nil ? 0:Int64(count),check:{[weak authority] in authority?.canPreview(self.actor)==true},onResponse:{http in
            if http.statusCode==401||http.statusCode==403{denied=true;throw PMProtectedError.rejected}
            let validator=http.value(forHTTPHeaderField:"ETag") ?? ""
            if offset==nil {guard http.statusCode==200,http.expectedContentLength>0,http.expectedContentLength<=Int64(2<<30),validator.hasPrefix("\""),validator.hasSuffix("\""),validator.count<=512 else{throw PMProtectedError.rejected};self.size=http.expectedContentLength;self.etag=validator;self.contentType=(http.value(forHTTPHeaderField:"Content-Type") ?? "application/octet-stream").components(separatedBy:";")[0].lowercased()}
            else {guard http.statusCode==206,http.value(forHTTPHeaderField:"Content-Range")=="bytes \(offset!)-\(offset!+Int64(count)-1)/\(self.size)",self.etag.isEmpty||validator==self.etag else{throw PMProtectedError.rejected}}
        },onData:{body.append($0)})
        let token=try authority.registerCancellation(actor,transport.cancel);defer{authority.unregisterCancellation(token)}
        do{_ = try transport.run(request)}catch{body.resetBytes(in:0..<body.count);if denied && !refreshed{refreshed=true;try refresh();return try self.request(offset:offset,count:count)};throw error}
        guard authority.canPreview(actor),offset==nil||body.count==count else{body.resetBytes(in:0..<body.count);throw PMProtectedError.rejected};return body
    }
    func read(_ offset:Int64,_ count:Int)throws->Data {lock.lock();defer{lock.unlock()};guard offset>=0,count>0,count<=1<<20,offset<=size-Int64(count) else{throw PMProtectedError.invalidInput};return try request(offset:offset,count:count)}
}
