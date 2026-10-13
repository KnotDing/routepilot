import Foundation

@main
struct LocalizationBenchmark {
    static func main() throws {
        guard CommandLine.arguments.count == 3 else { fatalError("provide ui-en.json and errors-zh.json") }
        let english = try JSONDecoder().decode([String: String].self, from: Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1])))
        let errors = try JSONDecoder().decode([String: String].self, from: Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[2])))
        let cases = [("known-Chinese", "设置", "zh-CN"), ("known-English", "设置", "en"), ("unmatched-Chinese", "测试状态 fixture 123", "zh-CN")]
        for (name, text, locale) in cases {
            var checksum = 0
            let started = ProcessInfo.processInfo.systemUptime
            for _ in 0..<200 {
                checksum += NativeText.translate(text, locale: locale, english: english, errors: errors).utf8.count
            }
            let microseconds = (ProcessInfo.processInfo.systemUptime - started) * 1_000_000 / 200
            print("\(name): \(microseconds) us/call; checksum=\(checksum)")
        }
    }
}
