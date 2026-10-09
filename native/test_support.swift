import Foundation

// Standalone native checks do not start AppDelegate, a backend or update agent.
struct AppFailure: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}
func nativeCheck(_ condition: @autoclosure () throws -> Bool, _ message: String) throws {
    guard try condition() else { throw AppFailure(message: message) }
}
func nativeReject(_ message: String, _ work: () throws -> Void) throws {
    var rejected = false
    do { try work() } catch { rejected = true }
    try nativeCheck(rejected, message)
}
func nativeRelease(_ tag: String, body: String = "", draft: Bool = false, prerelease: Bool = false) throws -> GiteaRelease {
    let data = try JSONSerialization.data(withJSONObject: ["tag_name":tag,"assets":[],"body":body,"draft":draft,"prerelease":prerelease])
    return try JSONDecoder().decode(GiteaRelease.self, from: data)
}
