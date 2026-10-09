import AppKit
import Foundation
import WebKit
import Darwin

@MainActor
private final class NativeTestAppDelegate: NSObject, NSApplicationDelegate {
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
}

@main
struct NativeUITests {
    @MainActor static func main() {
        let app=NSApplication.shared
        let delegate=NativeTestAppDelegate();app.delegate=delegate;app.setActivationPolicy(.prohibited)
        Task { @MainActor in
            do { try await runChecks(); exit(0) }
            catch { fputs("FAIL native UI: " + error.localizedDescription + "\n", stderr); exit(1) }
        }
        app.run()
        withExtendedLifetime(delegate) {}
    }
    @MainActor static func runChecks() async throws {
        let app=NSApplication.shared
        let notesDirectory=URL(fileURLWithPath:CommandLine.arguments[1],isDirectory:true)
        let release=try nativeRelease("v1.0.0",body:"## Native release\n\n- **Rendered Markdown**\n\n[Release](https://gitea.knotding.com/knot/routepilot/releases)\n\n<script>window.untrusted=true</script>\n\n![tracker](https://example.invalid/image.png)")
        for selection in 0..<4 {
            let dialog=ReleaseUpdateDialog(release:release,assistantReady:true,resourceDirectory:notesDirectory)
            let task=Task { @MainActor in await dialog.run(activate:false) }
            try await Task.sleep(nanoseconds:50_000_000)
            guard let panel=app.windows.first(where:{$0.isVisible && $0.title==L("软件更新")}), let content=panel.contentView,
                  let view=content.subviews.compactMap({$0 as? WKWebView}).first,
                  let disclosure=content.subviews.compactMap({$0 as? NSButton}).first(where:{!$0.isBordered}) else { throw AppFailure(message:"native dialog did not open") }
            let collapsed=panel.frame.height
            disclosure.performClick(nil)
            try nativeCheck(panel.frame.height>collapsed && !view.isHidden,"notes disclosure did not expand")
            let deadline=Date().addingTimeInterval(12);var rendered=false
            while Date()<deadline {
                rendered=(try? await view.evaluateJavaScript("document.querySelector('h2')?.textContent === 'Native release'")) as? Bool == true
                if rendered { break };try await Task.sleep(nanoseconds:50_000_000)
            }
            try nativeCheck(rendered,"offline native Markdown remained blank")
            let safe=(try await view.evaluateJavaScript("!!document.querySelector('strong') && !document.querySelector('img') && !window.untrusted && document.documentElement.style.getPropertyValue('--notes-background').length > 0")) as? Bool
            try nativeCheck(safe==true,"native Markdown safety/appearance failed")
            let buttons=content.subviews.compactMap({$0 as? NSButton}).filter(\.isBordered).sorted{$0.tag<$1.tag}
            try nativeCheck(buttons.count==3 && buttons[0].frame.minY==buttons[2].frame.minY && buttons[0].frame.maxX<buttons[1].frame.minX,"update action layout changed")
            if selection==3 { panel.performClose(nil) } else { buttons[selection].performClick(nil) }
            let choice=await task.value
            switch selection {case 0:try nativeCheck(choice == .skip,"skip choice lost");case 2:try nativeCheck(choice == .install,"install choice lost");default:try nativeCheck(choice == .later,"later/close choice lost")}
        }
        let failure=ReleaseUpdateDialog(release:release,assistantReady:true,resourceDirectory:notesDirectory.appendingPathComponent("missing"))
        let pending=Task { @MainActor in await failure.run(activate:false) };try await Task.sleep(nanoseconds:50_000_000)
        guard let panel=app.windows.first(where:{$0.isVisible && $0.title==L("软件更新")}), let content=panel.contentView else { throw AppFailure(message:"failure dialog missing") }
        content.subviews.compactMap({$0 as? NSButton}).first(where:{!$0.isBordered})?.performClick(nil)
        try nativeCheck(content.subviews.compactMap({$0 as? NSTextField}).contains(where:{!$0.isHidden && $0.stringValue==L("更新说明加载失败，请关闭窗口后重新检查更新。")}),"failure stayed blank")
        panel.performClose(nil);_ = await pending.value

        let name="RoutePilot-window-test-"+UUID().uuidString
        defer{NSWindow.removeFrame(usingName:name)}
        func window()->NSWindow{let value=NSWindow(contentRect:NSRect(x:0,y:0,width:1150,height:780),styleMask:[.titled,.closable,.resizable],backing:.buffered,defer:false);value.isReleasedWhenClosed=false;value.minSize=NSSize(width:420,height:480);return value}
        let first=window();ManagementWindowFrame.restore(first,name:name);first.setContentSize(NSSize(width:720,height:520));ManagementWindowFrame.remember(first,name:name);let previous=first.frame;first.close()
        let second=window();ManagementWindowFrame.restore(second,name:name);try nativeCheck(abs(second.frame.width-previous.width)<1 && abs(second.frame.height-previous.height)<1,"management frame did not restore");second.close()
        print("PASS native UI: offline Markdown in real WKWebView, async choices, failure fallback, horizontal actions, window frame restoration")
    }
}
