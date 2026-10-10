import AppKit
import Darwin
import Foundation

@main
struct NativeCoreTests {
    @MainActor static func main() throws {
        for value in ["0.5.2","v1.0.0","v0.10.20"] { try nativeCheck(ReleaseUpdater.stableVersion(value) != nil, "stable version rejected") }
        for value in ["", "v", "1.2", "1.2.3.4", "01.2.3", "+1.2.3", "1.-1.0", "1.2.3-beta", "1.2.3+build", "release1.0.0", "١.٢.٣"] { try nativeCheck(ReleaseUpdater.stableVersion(value) == nil, "invalid stable version accepted: " + value) }
        for value in ["1.0.1-beta.1", "v1.2.3-beta.12"] { try nativeCheck(ReleaseUpdater.betaVersion(value) != nil, "beta version rejected") }
        for value in ["1.0.1-beta", "1.0.1-beta.0", "1.0.1-beta.01", "1.0.1-rc.1", "01.0.1-beta.1"] { try nativeCheck(ReleaseUpdater.betaVersion(value) == nil, "invalid beta version accepted: " + value) }
        for value in ["00:00", "03:00", "23:59"] { try nativeCheck(UpdateSchedule.isValidTime(value), "valid update time rejected: " + value) }
        for value in ["", "3:00", "24:00", "12:60", "１２:００"] { try nativeCheck(!UpdateSchedule.isValidTime(value), "invalid update time accepted: " + value) }
        let scheduleDate = Calendar.current.date(from: DateComponents(year: 2026, month: 10, day: 10, hour: 3, minute: 0))!
        try nativeCheck(UpdateSchedule.matches("03:00", at: scheduleDate), "configured local update time did not match")
        try nativeCheck(!UpdateSchedule.matches("03:01", at: scheduleDate), "scheduled update ran at the wrong time")
        try nativeCheck(UpdateSchedule.key(for: "03:00", at: scheduleDate) != UpdateSchedule.key(for: "04:00", at: scheduleDate), "changing the scheduled time did not create a distinct daily check")
        let betas = [try nativeRelease("v1.0.1-beta.1", prerelease:true), try nativeRelease("v1.0.1-beta.3", prerelease:true), try nativeRelease("v1.0.1-beta.2", draft:true, prerelease:true), try nativeRelease("v1.0.1-rc.1", prerelease:true), try nativeRelease("v1.1.0")]
        try nativeCheck(ReleaseUpdater.newestBeta(in: betas)?.tagName == "v1.0.1-beta.3", "latest valid beta not selected")
        let oldAssistant = try JSONDecoder().decode(UpdateAssistantStatus.self, from: Data(#"{"ready":true}"#.utf8))
        let currentAssistant = try JSONDecoder().decode(UpdateAssistantStatus.self, from: Data(#"{"ready":true,"protocol_version":1}"#.utf8))
        try nativeCheck(oldAssistant.protocolVersion == nil && currentAssistant.protocolVersion == 1, "update assistant protocol compatibility changed")
        try nativeCheck(ReleaseUpdater.newer(try nativeRelease("v1.0.0"),current:"0.5.2"), "major upgrade missed")
        try nativeCheck(ReleaseUpdater.newer(try nativeRelease("v1.0.1"),current:"1.0.1-beta.2"), "stable upgrade from beta missed")
        try nativeCheck(ReleaseUpdater.newerBeta(try nativeRelease("v1.0.1-beta.2",prerelease:true),current:"1.0.1-beta.1"), "newer beta was missed")
        try nativeCheck(ReleaseUpdater.newerBeta(try nativeRelease("v1.0.1-beta.1",prerelease:true),current:"1.0.0"), "beta upgrade from stable was missed")
        try nativeCheck(!ReleaseUpdater.newerBeta(try nativeRelease("v1.0.1-beta.1",prerelease:true),current:"1.0.1"), "beta offered over stable")
        try nativeCheck(!ReleaseUpdater.newer(try nativeRelease("v0.5.2"),current:"0.5.2"), "equal version offered")
        try nativeCheck(!ReleaseUpdater.newer(try nativeRelease("v1.0.0",draft:true),current:"0.5.2"), "draft offered")
        try nativeCheck(!ReleaseUpdater.newer(try nativeRelease("v1.0.0",prerelease:true),current:"0.5.2"), "prerelease offered")
        try nativeCheck(ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.5"), app:"0.5.5", backend:"0.5.4"), "old local backend not offered unified update")
        try nativeCheck(!ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.5"), app:"0.5.5", backend:"0.5.5"), "matching service reinstalled")
        try nativeCheck(!ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.4"), app:"0.5.5", backend:"0.5.3"), "repair downgraded app")
        try nativeCheck(!ReleaseUpdater.needsBackendUpdate(try nativeRelease("v0.5.5",draft:true), app:"0.5.5", backend:"0.5.4"), "draft repair accepted")
        try nativeCheck(ReleaseUpdater.needsBackendUpdate(try nativeRelease("v1.0.1-beta.2",prerelease:true), app:"1.0.1-beta.2", backend:"1.0.1-beta.1"), "beta backend repair was missed")
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


        for qps in [0.0, 1, 10, 100, 10000] {
            let speed = StatusBarFlowClock.multiplier(qps)
            try nativeCheck(speed >= 1 && speed <= 4, "animation speed out of bounds")
        }
        try nativeCheck(StatusBarFlowClock.multiplier(100) > StatusBarFlowClock.multiplier(1), "QPS did not accelerate flow")
        try nativeCheck(StatusBarFlowClock.multiplier(.nan) == 1 && StatusBarFlowClock.multiplier(.infinity) == 1, "invalid QPS accelerated flow")
        var clock = StatusBarFlowClock()
        _ = clock.advance(1); let first = clock.advance(1.1)
        clock.queriesPerSecond = 100
        let second = clock.advance(1.2)
        try nativeCheck(second > first && second - first > 0.1, "flow clock rewound or did not accelerate")
        try nativeCheck(clock.advance(1.15) == second, "backwards clock changed packet position")
        clock.reset(); try nativeCheck(clock.advance(10) == 0, "clock reset failed")
        var preference = StatusBarPreferences()
        try nativeCheck(preference.valid && preference.animation && preference.display == "none", "status defaults changed")
        preference.display = "metric"
        let sample: [String: Any] = ["queries_per_second": 12.6, "counters": ["Errors": 5]]
        try nativeCheck(preference.text(status: sample, health: [], bgp: [:], available: true) == "13/s", "QPS text wrong")
        try nativeCheck(preference.text(status: sample, health: [], bgp: [:], available: false) == "—", "offline displayed stale data")
        let cacheSample: [String: Any] = ["cache": ["decision": ["entries": 12, "hits": 30, "misses": 4], "response": ["entries": 8, "hits": 20, "misses": 2]]]
        for (metric, expected) in [("decision_cache_entries", "12"), ("decision_cache_hits", "30"), ("decision_cache_misses", "4"), ("response_cache_entries", "8"), ("response_cache_hits", "20"), ("response_cache_misses", "2")] {
            preference.metric = metric
            try nativeCheck(preference.valid && preference.text(status: cacheSample, health: [], bgp: [:], available: true) == expected, "cache status bar metric wrong: \(metric)")
        }
        preference.metric = "client_count"
        let clientSample: [String: Any] = ["client_statistics": ["stats": ["clients": 3, "queries": 42, "direct": 20, "surge": 15, "reject": 4, "unknown": 3, "errors": 2]]]
        try nativeCheck(preference.valid && preference.text(status: clientSample, health: [], bgp: [:], available: true) == "3", "client count status bar metric wrong")
        preference.metric = "client_queries"
        try nativeCheck(preference.valid && preference.text(status: clientSample, health: [], bgp: [:], available: true) == "42", "client query status bar metric wrong")
        preference.display = "health_state"
        try nativeCheck(preference.text(status: [:], health: [["name":"Surge API", "state":"DOWN"]], bgp: [:], available: true) == "!", "health state missing")
        preference.display = "health_value"
        try nativeCheck(preference.text(status: [:], health: [["name":"Surge API", "latency_ms":1.6]], bgp: [:], available: true) == "2ms", "health metric wrong")
        try nativeCheck(preference.text(status: [:], health: [], bgp: [:], available: true) == "—", "missing component fabricated value")
        preference.display = "unsafe"; try nativeCheck(!preference.valid, "invalid display accepted")

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
