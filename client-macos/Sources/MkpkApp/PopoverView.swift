import SwiftUI
import UniformTypeIdentifiers
import MkpkKit

// Palette from the mockup (mkpk client.dc.html).
enum Palette {
    static let accent = Color(hex: 0x4753C5)
    static let open = Color(hex: 0x34C77B)
    static let warn = Color(hex: 0xF2A33C)
    static let error = Color(hex: 0xF26257)
    static let idle = Color(hex: 0x9A9AA2)

    static func logColor(_ r: AppModel.LogResult) -> Color {
        switch r {
        case .open: return open
        case .closed: return Color(hex: 0x83838A)
        case .timeout: return warn
        case .error: return error
        case .sent: return accent
        case .unverified: return warn
        }
    }
}

/// Status → colour / label, shared by the row and the detail screen.
enum StatusUI {
    static func color(_ s: AppModel.ServiceStatus) -> Color {
        switch s {
        case .open: return Palette.open
        case .closed: return Color(hex: 0x83838A)
        case .error: return Palette.error
        case .unverified: return Palette.warn
        case .knocking, .checking: return Palette.accent
        case .unknown: return Palette.idle.opacity(0.5)
        }
    }
    /// A lock/question glyph reflecting the service's access state.
    static func icon(_ s: AppModel.ServiceStatus) -> String {
        switch s {
        case .unknown: return "questionmark.circle"
        case .checking, .knocking: return "lock.fill"
        case .open: return "lock.open.fill"
        case .closed: return "lock.fill"
        case .unverified: return "questionmark.circle.fill"
        case .error: return "exclamationmark.triangle.fill"
        }
    }
    @MainActor static func shortLabel(_ s: AppModel.ServiceStatus) -> String {
        switch s {
        case .unknown: return L("not checked", "не проверено")
        case .checking: return L("checking…", "проверка…")
        case .knocking: return L("knocking…", "стук…")
        case .open: return L("open", "открыто")
        case .closed: return L("closed", "закрыто")
        case .unverified: return L("unverified", "не подтв.")
        case .error: return L("error", "ошибка")
        }
    }
    @MainActor static func line(_ svc: AppModel.ServiceVM, now: Date) -> String {
        switch svc.status {
        case .unknown: return L("Not checked", "Не проверялось")
        case .checking: return L("Checking…", "Проверяем…")
        case .knocking: return L("Knocking…", "Стучимся…")
        case .open:
            if let until = svc.openUntil {
                let rem = formatRemaining(max(0, until.timeIntervalSince(now)))
                return L("Open · \(rem) left", "Открыто · ещё \(rem)")
            }
            return L("Open", "Открыто")
        case .closed: return L("Closed", "Закрыто")
        case .unverified: return L("Can't verify behind VPN/proxy", "Не проверить за VPN/прокси")
        case .error: return L("Error", "Ошибка")
        }
    }
    @MainActor static func formatRemaining(_ seconds: TimeInterval) -> String {
        let total = Int(seconds.rounded())
        let m = total / 60, s = total % 60
        return m > 0 ? L("\(m)m \(s)s", "\(m)м \(s)с") : L("\(s)s", "\(s)с")
    }
}

/// Brand logo — theme-matched tile (dark tile on dark, light tile on light) from
/// bundled resources; a soft shadow gives it definition against the matching bg.
enum Brand {
    static func logo(_ scheme: ColorScheme) -> Image {
        let name = scheme == .dark ? "icon-dark" : "icon-light"
        if let ns = loadPNG(name) { return Image(nsImage: ns) }
        return Image(systemName: "shield.lefthalf.filled")
    }

    /// Resolve a bundled PNG. In the packaged .app the icons are flattened into
    /// Contents/Resources (so the bundle has no unsealed content in its root and
    /// can be codesigned); `swift run` falls back to the SwiftPM resource bundle.
    private static func loadPNG(_ name: String) -> NSImage? {
        if let url = Bundle.main.url(forResource: name, withExtension: "png"),
           let ns = NSImage(contentsOf: url) { return ns }
        #if DEBUG
        if let url = Bundle.module.url(forResource: name, withExtension: "png"),
           let ns = NSImage(contentsOf: url) { return ns }
        #endif
        return nil
    }
}

struct AccentButton: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 12, weight: .semibold))
            .foregroundStyle(.white)
            .padding(.horizontal, 12).padding(.vertical, 5)
            .background(Palette.accent.opacity(configuration.isPressed ? 0.82 : 1), in: RoundedRectangle(cornerRadius: 7))
    }
}

struct OutlineButton: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 12))
            .foregroundStyle(.primary)
            .padding(.horizontal, 10).padding(.vertical, 5)
            .background(RoundedRectangle(cornerRadius: 7).fill(Color.primary.opacity(configuration.isPressed ? 0.12 : 0.05)))
            .overlay(RoundedRectangle(cornerRadius: 7).stroke(Color.primary.opacity(0.15)))
    }
}

struct DangerButton: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 12, weight: .semibold))
            .foregroundStyle(.white)
            .padding(.horizontal, 12).padding(.vertical, 5)
            .background(Palette.error.opacity(configuration.isPressed ? 0.82 : 1), in: RoundedRectangle(cornerRadius: 7))
    }
}

/// Sizes to its content's natural height, only scrolling once it exceeds the
/// proposed height. A plain ScrollView reports a minimal height, which stops the
/// panel from growing to fit — this keeps the dynamic vertical resize on every
/// screen (main / detail / settings).
struct FittingScroll<C: View>: View {
    @ViewBuilder var content: C
    var body: some View {
        ViewThatFits(in: .vertical) {
            content
            ScrollView { content }
        }
    }
}

struct PopoverView: View {
    @ObservedObject var model: AppModel
    @ObservedObject private var loc = L10n.shared   // re-render the tree on language change
    @Environment(\.colorScheme) private var colorScheme
    @State private var dropTargeted = false

    var body: some View {
        VStack(spacing: 0) {
            if let err = model.lastError {
                Text(err).font(.system(size: 11)).foregroundStyle(Palette.error)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 14).padding(.vertical, 6)
                    .background(Palette.error.opacity(0.12))
                    .onTapGesture { model.lastError = nil }
            }
            Group {
                switch model.screen {
                case .main: mainScreen
                case .detail(let id): detailScreen(id)
                case .settings: SettingsView(model: model)
                }
            }
        }
        .frame(width: 380)
        .frame(minHeight: 200, maxHeight: model.maxPanelHeight)
        .background(.regularMaterial)
        .overlay {
            if dropTargeted {
                RoundedRectangle(cornerRadius: 12)
                    .strokeBorder(Palette.accent, style: StrokeStyle(lineWidth: 2, dash: [6, 4]))
                    .padding(2)
                    .allowsHitTesting(false)
            }
        }
        .onDrop(of: [.fileURL], isTargeted: $dropTargeted) { providers in
            for p in providers {
                _ = p.loadObject(ofClass: URL.self) { url, _ in
                    if let url { Task { @MainActor in await model.importFile(url) } }
                }
            }
            return true
        }
    }

    // MARK: main

    private var mainScreen: some View {
        VStack(spacing: 0) {
            header
            Divider()
            if model.isEmpty {
                emptyState
            } else {
                FittingScroll { groupsContent }
            }
            Divider()
            VStack(alignment: .leading, spacing: 5) {
                HStack(spacing: 6) {
                    Image(systemName: "network").font(.system(size: 11)).foregroundStyle(.secondary)
                    if let ip = model.publicIP {
                        (Text(L("Opens for ", "Открывается для ")).foregroundColor(.secondary)
                         + Text(ip).font(.system(size: 11, weight: .semibold, design: .monospaced)).foregroundColor(.primary))
                            .font(.system(size: 11))
                    } else {
                        Text(L("Opens for your current IP", "Открывается для вашего текущего IP"))
                            .font(.system(size: 11)).foregroundStyle(.secondary)
                    }
                    Spacer(minLength: 0)
                    Button { model.refreshPublicIP() } label: {
                        // Fixed-size slot so swapping the spinner for the glyph
                        // (on refresh) doesn't nudge the footer layout.
                        ZStack {
                            if model.resolvingIP {
                                ProgressView().controlSize(.small)
                            } else {
                                Image(systemName: "arrow.clockwise").font(.system(size: 11, weight: .semibold))
                            }
                        }
                        .frame(width: 16, height: 16)
                    }
                    .buttonStyle(.plain).foregroundStyle(.secondary)
                    .help(L("Re-check external IP", "Перепроверить внешний IP")).disabled(model.resolvingIP)
                }
                if model.hasStaleOpenIP {
                    HStack(alignment: .top, spacing: 6) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 10)).foregroundStyle(Palette.error)
                        Text(L("IP changed — the open access is for your old address. Re-knock.",
                               "IP изменился — открытый доступ действует для старого адреса. Перестучите."))
                            .font(.system(size: 10)).foregroundStyle(Palette.error)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 14).padding(.vertical, 9)
        }
    }

    private var groupsContent: some View {
        VStack(alignment: .leading, spacing: 12) {
            ForEach(model.clientGroups) { client in
                ClientGroupView(model: model, client: client)
            }
        }
        .padding(12)
    }

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "tray.and.arrow.down")
                .font(.system(size: 30)).foregroundStyle(.secondary)
            Text(L("Import an invite", "Импортируйте инвайт")).font(.system(size: 14, weight: .semibold))
            Text(L("Drop a .mkpk here, open a file, or paste a blob from the clipboard.",
                   "Перетащите .mkpk сюда, откройте файл или вставьте блоб из буфера."))
                .font(.system(size: 11)).foregroundStyle(.secondary)
                .multilineTextAlignment(.center).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Button(L("Open file…", "Открыть файл…")) { model.openFilePanel() }.buttonStyle(AccentButton())
                Button(L("Paste blob", "Вставить блоб")) { model.pasteBlob() }.buttonStyle(OutlineButton())
            }
            .padding(.top, 2)
        }
        .padding(.horizontal, 28).padding(.vertical, 32)
        .frame(maxWidth: .infinity)
    }

    private var header: some View {
        HStack(spacing: 9) {
            Brand.logo(colorScheme)
                .resizable().interpolation(.high)
                .frame(width: 30, height: 30)
                .shadow(color: .black.opacity(colorScheme == .dark ? 0.55 : 0.22), radius: 3, x: 0, y: 1)
            VStack(alignment: .leading, spacing: 1) {
                Text("mkpk").font(.system(size: 13, weight: .semibold))
                Text("Knock first").font(.system(size: 11)).foregroundStyle(.secondary)
            }
            Spacer()
            Menu {
                Button(L("Open file…", "Открыть файл…")) { model.openFilePanel() }
                Button(L("Paste from clipboard", "Вставить из буфера")) { model.pasteBlob() }
            } label: {
                Image(systemName: "plus")
            }
            .menuStyle(.borderlessButton).menuIndicator(.hidden).fixedSize()
            Button { model.pinned.toggle() } label: {
                Image(systemName: model.pinned ? "pin.fill" : "pin")
            }
            .buttonStyle(.borderless)
            .foregroundStyle(model.pinned ? Palette.accent : .secondary)
            .help(L("Pin the window to drag a file in from Finder", "Закрепить окно, чтобы перетащить файл из Finder"))
            Button { model.showSettings() } label: { Image(systemName: "gearshape") }.buttonStyle(.borderless)
        }
        .padding(.horizontal, 14).padding(.vertical, 11)
    }

    // MARK: detail

    @ViewBuilder
    private func detailScreen(_ id: String) -> some View {
        if let svc = model.service(id: id) {
            ServiceDetailView(model: model, svc: svc)
        } else {
            // The invite was removed while its detail was open — fall back.
            Color.clear.onAppear { model.showMain() }
        }
    }
}

/// One client_id block: its identity header (with the trash that removes the
/// whole invite) and its router cards nested inside a subtly framed container,
/// so multiple identities read as visually distinct blocks.
struct ClientGroupView: View {
    @ObservedObject var model: AppModel
    let client: AppModel.ClientGroup
    @State private var confirmingDelete = false

    var body: some View {
        Group {
            if confirmingDelete { confirmView } else { normalView }
        }
        .padding(11)
        .background(RoundedRectangle(cornerRadius: 11).fill(Color.primary.opacity(0.04)))
        .overlay(RoundedRectangle(cornerRadius: 11).stroke(Color.primary.opacity(confirmingDelete ? 0.0 : 0.10)))
        .overlay(RoundedRectangle(cornerRadius: 11).stroke(confirmingDelete ? Palette.error.opacity(0.5) : .clear))
    }

    private var normalView: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 6) {
                Image(systemName: "person.text.rectangle").font(.system(size: 11)).foregroundStyle(.secondary)
                Text(client.clientID)
                    .font(.system(size: 12, weight: .semibold, design: .monospaced))
                    .lineLimit(1).truncationMode(.middle)
                Spacer()
                Button { confirmingDelete = true } label: {
                    Image(systemName: "trash").font(.system(size: 11))
                }
                .buttonStyle(.borderless).foregroundStyle(.secondary)
                .help(L("Delete invite", "Удалить инвайт"))
            }
            ForEach(client.routers) { group in
                RouterGroupView(model: model, group: group)
            }
        }
    }

    private var confirmView: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 6) {
                Image(systemName: "trash").font(.system(size: 12)).foregroundStyle(Palette.error)
                Text(L("Delete invite?", "Удалить инвайт?")).font(.system(size: 13, weight: .semibold))
                Spacer()
            }
            Text(verbatim: "client_id \(client.clientID)")
                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                .lineLimit(1).truncationMode(.middle)
            Text(L("The invite and all its routers (\(client.routers.count)) will be removed.",
                   "Инвайт и все его роутеры (\(client.routers.count)) будут удалены."))
                .font(.system(size: 11)).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Spacer()
                Button(L("Cancel", "Отмена")) { confirmingDelete = false }.buttonStyle(OutlineButton())
                Button(L("Delete", "Удалить")) {
                    Task { await model.remove(clientID: client.clientID) }
                }.buttonStyle(DangerButton())
            }
        }
    }
}

struct RouterGroupView: View {
    @ObservedObject var model: AppModel
    let group: AppModel.RouterGroup

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 7) {
                Circle().fill(group.reachable == false ? Palette.error : Palette.open).frame(width: 7, height: 7)
                Text(group.address).font(.system(size: 12, weight: .semibold, design: .monospaced))
                if group.clockWarn {
                    Text(L("clock", "часы")).font(.system(size: 10, weight: .medium))
                        .padding(.horizontal, 5).padding(.vertical, 1)
                        .background(Palette.warn.opacity(0.2)).foregroundStyle(Palette.warn).clipShape(Capsule())
                }
                Spacer()
            }
            VStack(spacing: 8) {
                ForEach(group.services) { svc in
                    ServiceRowView(model: model, svc: svc)
                }
            }
        }
    }
}

struct ServiceRowView: View {
    @ObservedObject var model: AppModel
    let svc: AppModel.ServiceVM

    private var inlineExpanded: Bool {
        model.detailVariant == .inline && model.expandedServiceID == svc.id
    }
    private var chevron: String {
        model.detailVariant == .inline ? (inlineExpanded ? "chevron.down" : "chevron.right") : "chevron.right"
    }

    var body: some View {
        VStack(spacing: 0) {
            row
                .padding(10)
            if inlineExpanded {
                Divider()
                VStack(alignment: .leading, spacing: 12) {
                    KeepOpenToggle(model: model, svc: svc)
                    KnockLogView(model: model, entries: model.logs[svc.id] ?? [])
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(10)
                .background(Color.primary.opacity(0.04))
            }
        }
        .background(RoundedRectangle(cornerRadius: 9).fill(Color(.textBackgroundColor).opacity(0.5)))
        .overlay(RoundedRectangle(cornerRadius: 9).stroke(Color.primary.opacity(0.08)))
        .clipShape(RoundedRectangle(cornerRadius: 9))
    }

    private var row: some View {
        HStack(spacing: 10) {
            Image(systemName: model.ipStale(svc) ? "exclamationmark.triangle.fill" : StatusUI.icon(svc.status))
                .font(.system(size: 12))
                .foregroundStyle(model.ipStale(svc) ? Palette.error : StatusUI.color(svc.status))
                .frame(width: 16)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(svc.name).font(.system(size: 13, weight: .semibold, design: .monospaced))
                        .lineLimit(1).truncationMode(.tail)
                    // Router address is already the group header — show only the
                    // service-specific check port here (or a "no check" hint).
                    // `verbatim` avoids SwiftUI localizing the Int (e.g. "10 800").
                    if svc.checkPort > 0 {
                        Text(verbatim: ":\(svc.checkPort)").font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(.secondary).fixedSize()
                    } else {
                        Text(L("no check", "без проверки")).font(.system(size: 10)).foregroundStyle(.secondary).fixedSize()
                    }
                    if model.isKeepOpen(svc.id) {
                        Image(systemName: "infinity").font(.system(size: 10, weight: .bold))
                            .foregroundStyle(Palette.accent).help(L("Keep-open is on", "Держать открытым включён")).fixedSize()
                    }
                }
                if model.ipStale(svc) {
                    Text(L("Open for the old IP — re-knock", "Открыто для старого IP — перестучите"))
                        .font(.system(size: 11)).foregroundStyle(Palette.error)
                } else {
                    Text(StatusUI.line(svc, now: model.now))
                        .font(.system(size: 11))
                        .foregroundStyle(svc.status == .open ? Palette.open
                                         : svc.status == .unverified ? Palette.warn : .secondary)
                }
            }
            Spacer(minLength: 6)
            if svc.status == .knocking || svc.status == .checking {
                ProgressView().controlSize(.small)
            } else {
                Button { model.knock(svc) } label: { Image(systemName: "hand.tap.fill").font(.system(size: 13, weight: .semibold)) }
                    .buttonStyle(AccentButton()).help(L("Knock and check", "Стукнуть и проверить"))
                if svc.canCheck {
                    Button { model.check(svc) } label: { Image(systemName: "arrow.clockwise").font(.system(size: 13, weight: .semibold)) }
                        .buttonStyle(OutlineButton()).help(L("Check the port only, no knock", "Только проверить порт, без стука"))
                }
            }
            Button { model.openDetails(svc) } label: {
                Image(systemName: chevron).font(.system(size: 11)).foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help(L("Details and log", "Детали и лог"))
        }
    }
}

/// "Keep open" toggle: re-knock shortly before the access expires. Needs the
/// invite's allowed_timeout (else disabled with a hint).
struct KeepOpenToggle: View {
    @ObservedObject var model: AppModel
    let svc: AppModel.ServiceVM

    var body: some View {
        let can = model.canKeepOpen(svc)
        HStack(alignment: .top, spacing: 10) {
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 5) {
                    Image(systemName: "infinity").font(.system(size: 11, weight: .bold))
                        .foregroundStyle(can ? Palette.accent : .secondary)
                    Text(L("Keep open", "Держать открытым")).font(.system(size: 12.5, weight: .medium))
                }
                Text(can ? L("Automatically re-knock shortly before access expires.",
                             "Автоматически перестукивать незадолго до истечения доступа.")
                         : L("Unavailable: the invite has no timeout — re-issue it.",
                             "Недоступно: инвайт без таймаута — перевыпустите инвайт."))
                    .font(.system(size: 10.5)).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 8)
            Toggle("", isOn: Binding(get: { model.isKeepOpen(svc.id) },
                                     set: { model.setKeepOpen(svc, $0) }))
                .labelsHidden().toggleStyle(.switch).tint(Palette.accent)
                .disabled(!can)
        }
    }
}

/// Recent knock/check attempts, shared by the inline expander and the detail screen.
struct KnockLogView: View {
    @ObservedObject var model: AppModel
    let entries: [AppModel.LogEntry]

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(L("RECENT KNOCKS", "ПОСЛЕДНИЕ СТУКИ"))
                .font(.system(size: 10, weight: .semibold)).tracking(0.6).foregroundStyle(.secondary)
            if entries.isEmpty {
                Text(L("No attempts yet", "Пока нет попыток")).font(.system(size: 11)).foregroundStyle(.secondary)
            } else {
                ForEach(entries) { e in
                    HStack(spacing: 8) {
                        Text(model.timeLabel(e.time))
                            .font(.system(size: 11, design: .monospaced)).foregroundStyle(.secondary)
                        Text(model.logLabel(e.result))
                            .font(.system(size: 11, weight: .semibold)).foregroundStyle(Palette.logColor(e.result))
                        Text(e.note).font(.system(size: 11)).foregroundStyle(.secondary)
                    }
                }
            }
        }
    }
}

struct ServiceDetailView: View {
    @ObservedObject var model: AppModel
    let svc: AppModel.ServiceVM

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Button { model.showMain() } label: { Image(systemName: "chevron.left").font(.system(size: 13, weight: .semibold)) }
                    .buttonStyle(.borderless).foregroundStyle(.secondary)
                Circle().fill(StatusUI.color(svc.status)).frame(width: 9, height: 9)
                Text(svc.name).font(.system(size: 14.5, weight: .bold, design: .monospaced))
                Spacer()
                Text(StatusUI.shortLabel(svc.status))
                    .font(.system(size: 11, weight: .semibold)).foregroundStyle(StatusUI.color(svc.status))
            }
            .padding(.horizontal, 14).padding(.vertical, 11)
            Divider()
            FittingScroll {
                VStack(alignment: .leading, spacing: 12) {
                    if model.ipStale(svc) {
                        HStack(alignment: .top, spacing: 6) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 11)).foregroundStyle(Palette.error)
                            Text(L("Your external IP changed — access is open for the old address. Knock again.",
                                   "Внешний IP изменился — доступ открыт для старого адреса. Стукните заново."))
                                .font(.system(size: 11)).foregroundStyle(Palette.error)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(10)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 9).fill(Palette.error.opacity(0.12)))
                    }
                    Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 14, verticalSpacing: 5) {
                        detailRow(L("Router", "Роутер"), svc.routerAddress, mono: true)
                        detailRow(L("Check", "Проверка"), svc.checkPort > 0 ? "\(svc.checkPort)" : L("check off", "выкл"), mono: true)
                        detailRow(L("Access for", "Доступ на"), model.ttlText(svc))
                        detailRow(L("Last knock", "Последний стук"), model.lastKnockText(for: svc.id))
                    }
                    .padding(11)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(RoundedRectangle(cornerRadius: 9).fill(Color(.textBackgroundColor).opacity(0.5)))
                    .overlay(RoundedRectangle(cornerRadius: 9).stroke(Color.primary.opacity(0.08)))

                    HStack(spacing: 6) {
                        if svc.status == .knocking || svc.status == .checking {
                            ProgressView().controlSize(.small).frame(maxWidth: .infinity)
                        } else {
                            Button(L("Knock", "Стук")) { model.knock(svc) }.buttonStyle(AccentButton()).frame(maxWidth: .infinity)
                            if svc.canCheck {
                                Button(L("Check", "Проверить")) { model.check(svc) }.buttonStyle(OutlineButton()).frame(maxWidth: .infinity)
                            }
                        }
                    }

                    KeepOpenToggle(model: model, svc: svc)
                        .padding(11)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 9).fill(Color(.textBackgroundColor).opacity(0.5)))
                        .overlay(RoundedRectangle(cornerRadius: 9).stroke(Color.primary.opacity(0.08)))

                    KnockLogView(model: model, entries: model.logs[svc.id] ?? [])
                }
                .padding(14)
            }
        }
    }

    private func detailRow(_ label: String, _ value: String, mono: Bool = false) -> some View {
        GridRow {
            Text(label).font(.system(size: 11.5)).foregroundStyle(.secondary)
            Text(value).font(.system(size: 11.5, design: mono ? .monospaced : .default))
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

struct SettingsView: View {
    @ObservedObject var model: AppModel
    @ObservedObject private var loc = L10n.shared

    /// "mkpk · Knock first · <version>" — version present only in the .app bundle.
    static var aboutLine: String {
        let base = "mkpk · Knock first"
        if let v = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String, !v.isEmpty {
            return base + " · " + v
        }
        return base
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Button { model.showMain() } label: { Image(systemName: "chevron.left").font(.system(size: 13, weight: .semibold)) }
                    .buttonStyle(.borderless).foregroundStyle(.secondary)
                Text(L("Settings", "Настройки")).font(.system(size: 14.5, weight: .bold))
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 11)
            Divider()
            FittingScroll {
                VStack(alignment: .leading, spacing: 14) {
                    section(L("LANGUAGE", "ЯЗЫК")) {
                        card {
                            Picker("", selection: $loc.language) {
                                ForEach(AppLanguage.allCases) { lang in
                                    Text(lang.title).tag(lang)
                                }
                            }
                            .pickerStyle(.segmented).labelsHidden()
                        }
                    }

                    section(L("GENERAL", "ОБЩЕЕ")) {
                        card {
                            toggleRow(L("Launch at login", "Автозапуск при входе"),
                                      L("Start mkpk when you log in.", "Запускать mkpk при входе в систему."),
                                      isOn: $model.launchAtLogin)
                            Divider()
                            toggleRow(L("Notifications", "Уведомления"),
                                      L("Notify when access opens or closes.", "Сообщать, когда доступ открылся или закрылся."),
                                      isOn: $model.notificationsEnabled)
                        }
                    }

                    section(L("SYNC", "СИНХРОНИЗАЦИЯ")) {
                        card {
                            toggleRow("iCloud",
                                      L("Sync invites via iCloud Keychain. Requires a provisioning profile (not yet available).",
                                        "Синхронизировать инвайты через iCloud Keychain. Требуется provisioning profile (пока недоступно)."),
                                      isOn: $model.iCloudSync)
                        }
                    }

                    section(L("DIAGNOSTICS", "ДИАГНОСТИКА")) {
                        card {
                            toggleRow(L("Verbose logging", "Подробное логирование"),
                                      L("Log checks, IP resolution and network changes to the system log (Console.app · subsystem ru.eg23.mkpk.client). For troubleshooting.",
                                        "Писать проверки, резолв IP и смену сети в системный лог (Console.app · subsystem ru.eg23.mkpk.client). Для диагностики."),
                                      isOn: $model.verboseLogging)
                        }
                    }

                    section(L("SERVICE DETAILS", "ДЕТАЛИ СЕРВИСА")) {
                        card {
                            VStack(alignment: .leading, spacing: 8) {
                                Picker("", selection: $model.detailVariant) {
                                    Text(L("Inline", "Инлайн")).tag(AppModel.DetailVariant.inline)
                                    Text(L("Screen", "Экран")).tag(AppModel.DetailVariant.screen)
                                }
                                .pickerStyle(.segmented).labelsHidden()
                                Text(L("How to open a service's details and log: expand the row in place, or a separate screen.",
                                       "Как открывать детали и лог сервиса: раскрытием строки на месте или отдельным экраном."))
                                    .font(.system(size: 10.5)).foregroundStyle(.secondary)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                        }
                    }

                    HStack(spacing: 8) {
                        Brand.logo(.dark)
                            .resizable().interpolation(.high).frame(width: 16, height: 16)
                        Text(Self.aboutLine).font(.system(size: 11)).foregroundStyle(.secondary)
                        Button("· GitHub") {
                            if let url = URL(string: "https://github.com/LazyGatto/mikrotik-psk-knock") {
                                NSWorkspace.shared.open(url)
                            }
                        }
                        .buttonStyle(.borderless).font(.system(size: 11)).foregroundStyle(.secondary)
                        .help(L("Project page, releases and docs", "Страница проекта, релизы и документация"))
                        Spacer()
                        if let check = model.checkForUpdates {
                            Button(L("Check for updates", "Проверить обновления")) { check() }
                                .buttonStyle(OutlineButton())
                                .help(L("Check for a new mkpk version", "Проверить, вышла ли новая версия mkpk"))
                        }
                        Button(L("Quit", "Выйти")) { model.quit() }
                            .buttonStyle(OutlineButton())
                            .help(L("Quit mkpk (or right-click the icon)", "Завершить mkpk (или правый клик по иконке)"))
                    }
                    .padding(.top, 2)
                }
                .padding(14)
            }
        }
    }

    @ViewBuilder
    private func section<C: View>(_ title: String, @ViewBuilder _ content: () -> C) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title).font(.system(size: 10, weight: .semibold)).tracking(0.6).foregroundStyle(.secondary)
            content()
        }
    }

    @ViewBuilder
    private func card<C: View>(@ViewBuilder _ content: () -> C) -> some View {
        VStack(alignment: .leading, spacing: 8) { content() }
            .padding(11)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 9).fill(Color(.textBackgroundColor).opacity(0.5)))
            .overlay(RoundedRectangle(cornerRadius: 9).stroke(Color.primary.opacity(0.08)))
    }

    private func toggleRow(_ title: String, _ subtitle: String, isOn: Binding<Bool>) -> some View {
        HStack(alignment: .top, spacing: 10) {
            VStack(alignment: .leading, spacing: 1) {
                Text(title).font(.system(size: 12.5, weight: .medium))
                Text(subtitle).font(.system(size: 10.5)).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 8)
            Toggle("", isOn: isOn).labelsHidden().toggleStyle(.switch).tint(Palette.accent)
        }
    }
}

extension Color {
    init(hex: UInt32) {
        self.init(.sRGB,
                  red: Double((hex >> 16) & 0xFF) / 255,
                  green: Double((hex >> 8) & 0xFF) / 255,
                  blue: Double(hex & 0xFF) / 255,
                  opacity: 1)
    }
}
