import AppKit
import Darwin
import Foundation

@main
struct NativeCoreTests {
    @MainActor static func main() throws {
        for value in ["0.5.2","v1.0.0","v0.10.20"] { try nativeCheck(ReleaseUpdater.stableVersion(value) != nil, "stable version rejected") }
        for value in ["", "v", "1.2", "1.2.3.4", "01.2.3", "+1.2.3", "1.-1.0", "1.2.3-beta", "1.2.3+build", "release1.0.0", "١.٢.٣"] { try nativeCheck(ReleaseUpdater.stableVersion(value) == nil, "invalid stable version accepted: " + value) }
        try nativeCheck(ReleaseUpdater.newer(try nativeRelease("v1.0.0"),current:"0.5.2"), "major upgrade missed")
        try nativeCheck(!ReleaseUpdater.newer(try nativeRelease("v0.5.2"),current:"0.5.2"), "equal version offered")
        try nativeCheck(!ReleaseUpdater.newer(try nativeRelease("v1.0.0",draft:true),current:"0.5.2"), "draft offered")
        try nativeCheck(!ReleaseUpdater.newer(try nativeRelease("v1.0.0",prerelease:true),current:"0.5.2"), "prerelease offered")
        try nativeCheck(ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.5"), app:"0.5.5", backend:"0.5.4"), "old local backend not offered unified update")
        try nativeCheck(!ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.5"), app:"0.5.5", backend:"0.5.5"), "matching service reinstalled")
        try nativeCheck(!ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.4"), app:"0.5.5", backend:"0.5.3"), "repair downgraded app")
        try nativeCheck(!ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.5",draft:true), app:"0.5.5", backend:"0.5.4"), "draft repair accepted")
        for address in ["https://gitea.knotding.com/a","https://gitea.knotding.com:443/a"] { try nativeCheck(ReleaseUpdater.trustedDownloadURL(URL(string:address)!), "trusted origin rejected") }
        for address in ["http://gitea.knotding.com/a","https://evil.test/a","https://gitea.knotding.com:8443/a","https://user:pass@gitea.knotding.com/a","https://gitea.knotding.com/a#fragment"] { try nativeCheck(!ReleaseUpdater.trustedDownloadURL(URL(string:address)!), "untrusted origin accepted") }
        let digest=String(repeating:"a",count:64), name="RoutePilot-macos-arm64.zip"
        try nativeCheck(try ReleaseUpdater.checksum(digest.uppercased()+" *"+name,archiveName:name) == digest, "checksum normalization failed")
        for text in ["", "invalid "+name, digest+" other.zip",digest+" "+name+"\n"+digest+" "+name] { try nativeReject("bad checksum accepted") { _ = try ReleaseUpdater.checksum(text,archiveName:name) } }

        var flow=StatusBarPacketFlow(), randomCalls=0
        flow.setTraffic(direct:7,surge:3)
        let direct=flow.route(for:0,random:{randomCalls+=1;return 0.2})
        try nativeCheck(direct == .direct, "packet took wrong weighted branch")
        flow.setTraffic(direct:0,surge:1)
        try nativeCheck(flow.route(for:0,random:{randomCalls+=1;return 0.9}) == .direct && randomCalls==1, "packet changed direction mid-flight")
        try nativeCheck(flow.route(for:1,random:{0}) == .surge, "new probability did not apply")
        flow.setTraffic(direct:1,surge:0);try nativeCheck(flow.route(for:2,random:{0.999}) == .direct, "100% direct not respected")
        flow.setTraffic(direct:.nan,surge:.infinity);try nativeCheck(flow.directProbability == 0.5, "missing counters did not use equal odds")
        flow.setTraffic(direct:73,surge:27);var seed:UInt64=42,count=0
        for cycle in 3..<10003 { let branch=flow.route(for:cycle,random:{seed=seed &* 6364136223846793005 &+ 1;return Double(seed >> 11)/9007199254740992});if branch == .direct { count+=1 } }
        try nativeCheck(abs(Double(count)/10000-0.73)<0.02, "weighted packet distribution diverged")
        let upper=StatusBarVisualState.flowImage(progress:0.7,branch:.direct), lower=StatusBarVisualState.flowImage(progress:0.7,branch:.surge)
        try nativeCheck(upper.isTemplate && upper.size==NSSize(width:18,height:18), "icon geometry/template changed")
        try nativeCheck(upper.tiffRepresentation != lower.tiffRepresentation, "both branches render the same packet")
        try nativeCheck(StatusBarVisualState.flowImage(progress:0.1,branch:.direct).tiffRepresentation == StatusBarVisualState.flowImage(progress:0.1,branch:.surge).tiffRepresentation, "single input packet was duplicated")

        let files=FileManager.default, directory=files.temporaryDirectory.appendingPathComponent("RoutePilot-credentials-test-"+UUID().uuidString)
        try files.createDirectory(at:directory,withIntermediateDirectories:false);defer{try? files.removeItem(at:directory)}
        let store=LocalCredentials(directory:directory), credential=LoginCredential(username:"fixture",password:"fixture-password")
        try nativeCheck(try store.load() == nil,"missing credentials not empty")
        try store.save(credential,filename:"login.json");try nativeCheck(try store.load()?.password==credential.password,"credential round trip failed")
        try nativeCheck((try files.attributesOfItem(atPath:directory.path)[.posixPermissions] as? NSNumber)?.intValue==0o700,"credential directory permissions")
        let file=directory.appendingPathComponent("login.json")
        try nativeCheck((try files.attributesOfItem(atPath:file.path)[.posixPermissions] as? NSNumber)?.intValue==0o600,"credential file permissions")
        try store.save(LoginCredential(username:"remote",password:"remote-password"),filename:"remote-fixture.json")
        try nativeCheck(try store.load()?.username=="fixture","remote save replaced local credential")
        try nativeReject("path traversal credential accepted") { try store.save(credential,filename:"../outside.json") }
        try files.removeItem(at:file);let outside=directory.appendingPathComponent("outside");try Data("outside-content".utf8).write(to:outside)
        try files.createSymbolicLink(at:file,withDestinationURL:outside)
        try nativeReject("symlink credential read") { _=try store.load() }
        try store.save(credential,filename:"login.json");try nativeCheck(try String(contentsOf:outside,encoding:.utf8)=="outside-content","save followed symlink")
        let hard=directory.appendingPathComponent("hard.json");try files.linkItem(at:file,to:hard)
        try nativeReject("hardlinked credential read") { _=try store.load() };try files.removeItem(at:hard)
        try Data(repeating:65,count:16385).write(to:file);try nativeReject("oversized credential read"){_=try store.load()}
        try Data("invalid JSON".utf8).write(to:file);try nativeReject("invalid credential JSON read"){_=try store.load()}
        try store.delete();try store.delete();try nativeCheck(try store.load()==nil,"credential delete failed")
        let linked=directory.appendingPathComponent("linked");try files.createSymbolicLink(at:linked,withDestinationURL:directory)
        try nativeReject("symlink credential directory accepted"){_=try LocalCredentials(directory:linked).load()}
        print("PASS native core: stable versions, trusted origins, checksums, weighted packets, immutable paths, private credential storage")
    }
}
