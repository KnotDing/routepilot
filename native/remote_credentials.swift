import Foundation
import WebKit

@MainActor
extension AppDelegate {
    private func credentialOrigin(_ raw: String) -> String? {
        guard raw.utf8.count <= 512, let url = URLComponents(string: raw),
              let scheme = url.scheme?.lowercased(), ["http", "https"].contains(scheme),
              let rawHost = url.host, !rawHost.isEmpty, url.user == nil, url.password == nil,
              ["", "/"].contains(url.path), url.query == nil, url.fragment == nil else { return nil }
        let host = rawHost.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        let port = url.port ?? (scheme == "https" ? 443 : 80)
        guard (1...65535).contains(port) else { return nil }
        return scheme + "://" + (host.contains(":") ? "[" + host + "]" : host) + ":" + String(port)
    }
    private func credentialPage(_ view: WKWebView, base: URL) -> Bool {
        guard webView === view, let page = view.url else { return false }
        return page.scheme == base.scheme && page.host == base.host && page.port == base.port && client.base == base
    }
    func handleRemoteCredentials(_ values: [String: String]) async {
        guard let view = webView, let action = values["action"], ["save", "login", "forget"].contains(action),
              let requestID = values["requestID"], (1...80).contains(requestID.count), requestID.allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "-") }),
              let id = values["backendID"], (1...64).contains(id.count), id.allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "-" || $0 == "_") }),
              let address = values["backendURL"], let origin = credentialOrigin(address) else { return }
        let base = client.base
        let selectedStore = CredentialVault.storage
        var reply: [String: Any] = ["requestID": requestID, "backendID": id, "backendURL": address]
        do {
            guard credentialPage(view, base: base) else { return }
            // Resolve the ID against the local authenticated browser's profiles;
            // a page cannot request credentials for an arbitrary supplied URL.
            let resolved = try await view.callAsyncJavaScript("""
                const response = await fetch('/api/v1/management/backends', {credentials:'same-origin'});
                if (!response.ok) throw new Error('无法读取本机后端连接，请重新登录本机服务');
                const profiles = await response.json();
                return profiles.find(profile => profile.id === id) || null;
                """, arguments: ["id": id], in: nil, contentWorld: .page) as? [String: Any]
            guard credentialPage(view, base: base), let resolved, let configuredURL = resolved["url"] as? String,
                  credentialOrigin(configuredURL) == origin else { throw AppFailure(message: "远程后端地址已变更，请重新连接后登录") }
            let scope = "remote:" + id + ":" + origin
            switch action {
            case "save":
                guard let username = values["username"], !username.isEmpty, username.utf8.count <= 256,
                      let password = values["password"], !password.isEmpty, password.utf8.count <= 1024 else { throw AppFailure(message: "登录凭证无效") }
                let authenticated = try await view.callAsyncJavaScript("""
                    const response = await fetch('/api/v1/management/relay/' + encodeURIComponent(id) + '/session', {credentials:'same-origin'});
                    return response.ok && (await response.json()).authenticated === true;
                    """, arguments: ["id": id], in: nil, contentWorld: .page) as? Bool
                guard credentialPage(view, base: base), authenticated == true else { throw AppFailure(message: "请先成功登录远程后端，再保存凭证") }
                try CredentialVault.save(username: username, password: password, to: selectedStore, scope: scope)
                reply["ok"] = true
            case "forget":
                try CredentialVault.forget(scope: scope, from: selectedStore)
                reply["ok"] = true
            default:
                guard let credential = try CredentialVault.saved(scope: scope, from: selectedStore) else {
                    reply["missing"] = true
                    break
                }
                // Passwords stay out of page storage and reply events. A single
                // scoped fetch uses them only for the selected remote login.
                let result = try await view.callAsyncJavaScript("""
                    const active = () => window.routepilotActiveBackend?.id === id && window.routepilotActiveBackend?.url === address;
                    if (!active()) return {cancelled:true};
                    const headers = {'Content-Type':'application/json'};
                    const controller = new AbortController();
                    const timer = setTimeout(() => controller.abort(), 15000);
                    try {
                        const options = {credentials:'same-origin',signal:controller.signal};
                        const statusResponse = await fetch('/api/v1/status', options);
                        if (!statusResponse.ok || !(await statusResponse.json()).remote_credential_origin_binding)
                            throw new Error('请更新本机后台服务以启用远程自动登录');
                        const localResponse = await fetch('/api/v1/session', options);
                        if (!localResponse.ok) throw new Error('无法读取本机会话，请重新登录本机服务');
                        const local = await localResponse.json();
                        if (!local.authenticated) return {localExpired:true};
                        if (!active()) return {cancelled:true};
                        headers['X-CSRF-Token'] = local.csrf;
                        headers['X-RoutePilot-Expected-Backend'] = address;
                        const response = await fetch('/api/v1/management/relay/' + encodeURIComponent(id) + '/login', {...options,method:'POST',headers,body:JSON.stringify({username,password})});
                        if (response.ok) return {session:await response.json()};
                        if (response.status === 401 && response.headers.get('X-RoutePilot-Backend') !== 'remote') return {localExpired:true};
                        return {status:response.status,error:(await response.text()).trim()};
                    } finally { clearTimeout(timer); }
                    """, arguments: ["id": id, "address": address, "username": credential.username, "password": credential.password], in: nil, contentWorld: .page) as? [String: Any] ?? [:]
                guard credentialPage(view, base: base) else { return }
                if result["status"] as? Int == 401 {
                    try CredentialVault.forget(scope: scope, from: selectedStore)
                    reply["error"] = L("保存的远程登录凭证已失效，请重新登录一次")
                } else { result.forEach { reply[$0.key] = $0.value } }
            }
        } catch { let details = (error as NSError).userInfo["WKJavaScriptExceptionMessage"] as? String ?? error.localizedDescription; reply["error"] = L(details.hasPrefix("Error: ") ? String(details.dropFirst(7)) : details) }
        guard credentialPage(view, base: base), let data = try? JSONSerialization.data(withJSONObject: reply), let json = String(data: data, encoding: .utf8) else { return }
        _ = try? await view.evaluateJavaScript("window.dispatchEvent(new CustomEvent('routepilot-credentials-result',{detail:" + json + "}));")
    }
}
