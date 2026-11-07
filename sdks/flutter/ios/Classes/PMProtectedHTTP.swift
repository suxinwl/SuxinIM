import Foundation
import Security
import CryptoKit

/// No cookie/cache/redirect/global trust override. Responses are delivered in
/// bounded RAM blocks to the caller's authorized ciphertext sink.
@available(iOS 13.0, *)
final class PMProtectedHTTP:NSObject,URLSessionDataDelegate,URLSessionTaskDelegate {
    private let state=NSLock(),deadline:TimeInterval
    private var session:URLSession!,task:URLSessionDataTask?,done=false,head=false
    private var received:Int64=0,error:Error?,response:HTTPURLResponse?
    private let semaphore=DispatchSemaphore(value:0)
    private let maximum:Int64,check:()->Bool,onResponse:(HTTPURLResponse)throws->Void,onData:(Data)throws->Void
    init(timeout:TimeInterval,maximum:Int64,check:@escaping()->Bool,onResponse:@escaping(HTTPURLResponse)throws->Void,onData:@escaping(Data)throws->Void){self.deadline=timeout;self.maximum=maximum;self.check=check;self.onResponse=onResponse;self.onData=onData;super.init()}
    func run(_ request:URLRequest)throws->HTTPURLResponse {
        guard check() else{throw PMProtectedError.rejected}
        let config=URLSessionConfiguration.ephemeral;config.urlCache=nil;config.httpCookieStorage=nil;config.httpShouldSetCookies=false;config.requestCachePolicy = .reloadIgnoringLocalCacheData;config.timeoutIntervalForRequest=deadline;config.timeoutIntervalForResource=deadline
        head=request.httpMethod=="HEAD";session=URLSession(configuration:config,delegate:self,delegateQueue:nil);let task=session.dataTask(with:request);state.lock();guard !done else{state.unlock();session.invalidateAndCancel();throw PMProtectedError.cancelled};self.task=task;state.unlock();task.resume()
        defer{session.invalidateAndCancel()}
        guard semaphore.wait(timeout:.now()+deadline) == .success else{cancel();throw PMProtectedError.network}
        state.lock();let failure=error,http=response;state.unlock();if let failure=failure{throw failure};guard check(),let http=http else{throw PMProtectedError.rejected};return http
    }
    func cancel(){state.lock();let current=task;state.unlock();finish(PMProtectedError.cancelled);current?.cancel()}
    private func active()->Bool{state.lock();defer{state.unlock()};return !done}
    private func finish(_ failure:Error?){state.lock();if done{state.unlock();return};done=true;error=failure;state.unlock();semaphore.signal()}
    func urlSession(_ session:URLSession,dataTask:URLSessionDataTask,didReceive response:URLResponse,completionHandler:@escaping(URLSession.ResponseDisposition)->Void){
        do{guard active(),check(),let http=response as? HTTPURLResponse else{throw PMProtectedError.rejected};try onResponse(http);guard active(),head||response.expectedContentLength<=maximum else{throw PMProtectedError.rejected};state.lock();self.response=http;state.unlock();completionHandler(.allow)}catch{finish(error);completionHandler(.cancel)}
    }
    func urlSession(_ session:URLSession,dataTask:URLSessionDataTask,didReceive data:Data){guard active() else{return};do{guard check(),Int64(data.count)<=maximum-received else{throw PMProtectedError.rejected};try onData(data);received+=Int64(data.count);guard active(),check() else{throw PMProtectedError.rejected}}catch{finish(error);dataTask.cancel()}}
    func urlSession(_ session:URLSession,task:URLSessionTask,didCompleteWithError error:Error?){finish(error)}
    func urlSession(_ session:URLSession,task:URLSessionTask,willPerformHTTPRedirection response:HTTPURLResponse,newRequest request:URLRequest,completionHandler:@escaping(URLRequest?)->Void){completionHandler(nil)}
    func urlSession(_ session:URLSession,didReceive challenge:URLAuthenticationChallenge,completionHandler:@escaping(URLSession.AuthChallengeDisposition,URLCredential?)->Void){Self.trust(challenge,completionHandler)}
    static func trust(_ challenge:URLAuthenticationChallenge,_ complete:@escaping(URLSession.AuthChallengeDisposition,URLCredential?)->Void){
        guard challenge.protectionSpace.host=="192.168.10.11" else{complete(.performDefaultHandling,nil);return}
        guard challenge.protectionSpace.port==9885,challenge.protectionSpace.authenticationMethod==NSURLAuthenticationMethodServerTrust,let trust=challenge.protectionSpace.serverTrust,
          let url=Bundle(for:PMProtectedHTTP.self).url(forResource:"flutter_openim_sdk_privacy",withExtension:"bundle"),let bundle=Bundle(url:url),let certURL=bundle.url(forResource:"puty_enterprise_root_ca",withExtension:"der"),let der=try? Data(contentsOf:certURL),SHA256.hash(data:der).map({String(format:"%02x",$0)}).joined()=="836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69",let root=SecCertificateCreateWithData(nil,der as CFData) else{complete(.cancelAuthenticationChallenge,nil);return}
        SecTrustSetAnchorCertificates(trust,[root] as CFArray);SecTrustSetAnchorCertificatesOnly(trust,true);SecTrustSetPolicies(trust,SecPolicyCreateSSL(true,"192.168.10.11" as CFString))
        guard SecTrustEvaluateWithError(trust,nil),SecTrustGetCertificateCount(trust)==2,let final=SecTrustGetCertificateAtIndex(trust,1),SecCertificateCopyData(final) as Data==der else{complete(.cancelAuthenticationChallenge,nil);return};complete(.useCredential,URLCredential(trust:trust))
    }
}
