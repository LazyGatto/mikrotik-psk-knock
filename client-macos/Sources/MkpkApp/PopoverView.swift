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
        case .error: return "exclamationmark.triangle.fill"
        }
    }
    static func shortLabel(_ s: AppModel.ServiceStatus) -> String {
        switch s {
        case .unknown: return "не проверено"
        case .checking: return "проверка…"
        case .knocking: return "стук…"
        case .open: return "открыто"
        case .closed: return "закрыто"
        case .error: return "ошибка"
        }
    }
    static func line(_ svc: AppModel.ServiceVM, now: Date) -> String {
        switch svc.status {
        case .unknown: return "Не проверялось"
        case .checking: return "Проверяем…"
        case .knocking: return "Стучимся…"
        case .open:
            if let until = svc.openUntil {
                return "Открыто · ещё \(formatRemaining(max(0, until.timeIntervalSince(now))))"
            }
            return "Открыто"
        case .closed: return "Закрыто"
        case .error: return "Ошибка"
        }
    }
    static func formatRemaining(_ seconds: TimeInterval) -> String {
        let total = Int(seconds.rounded())
        let m = total / 60, s = total % 60
        return m > 0 ? "\(m)м \(s)с" : "\(s)с"
    }
}

/// Brand logo — theme-matched tile (dark tile on dark, light tile on light) from
/// bundled resources; a soft shadow gives it definition against the matching bg.
enum Brand {
    static func logo(_ scheme: ColorScheme) -> Image {
        let name = scheme == .dark ? "icon-dark" : "icon-light"
        if let url = Bundle.module.url(forResource: name, withExtension: "png"),
           let ns = NSImage(contentsOf: url) {
            return Image(nsImage: ns)
        }
        return Image(systemName: "shield.lefthalf.filled")
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
            HStack(spacing: 6) {
                Image(systemName: "network").font(.system(size: 11)).foregroundStyle(.secondary)
                if let ip = model.publicIP {
                    (Text("Открывается для ").foregroundColor(.secondary)
                     + Text(ip).font(.system(size: 11, weight: .semibold, design: .monospaced)).foregroundColor(.primary))
                        .font(.system(size: 11))
                } else {
                    Text("Открывается для вашего текущего IP")
                        .font(.system(size: 11)).foregroundStyle(.secondary)
                }
                Spacer(minLength: 0)
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
            Text("Импортируйте инвайт").font(.system(size: 14, weight: .semibold))
            Text("Перетащите .mkpk сюда, откройте файл или вставьте блоб из буфера.")
                .font(.system(size: 11)).foregroundStyle(.secondary)
                .multilineTextAlignment(.center).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Button("Открыть файл…") { model.openFilePanel() }.buttonStyle(AccentButton())
                Button("Вставить блоб") { model.pasteBlob() }.buttonStyle(OutlineButton())
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
                Button("Открыть файл…") { model.openFilePanel() }
                Button("Вставить из буфера") { model.pasteBlob() }
            } label: {
                Image(systemName: "plus")
            }
            .menuStyle(.borderlessButton).menuIndicator(.hidden).fixedSize()
            Button { model.pinned.toggle() } label: {
                Image(systemName: model.pinned ? "pin.fill" : "pin")
            }
            .buttonStyle(.borderless)
            .foregroundStyle(model.pinned ? Palette.accent : .secondary)
            .help("Закрепить окно, чтобы перетащить файл из Finder")
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
                .help("Удалить инвайт")
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
                Text("Удалить инвайт?").font(.system(size: 13, weight: .semibold))
                Spacer()
            }
            Text(verbatim: "client_id \(client.clientID)")
                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                .lineLimit(1).truncationMode(.middle)
            Text(verbatim: "Инвайт и все его роутеры (\(client.routers.count)) будут удалены.")
                .font(.system(size: 11)).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Spacer()
                Button("Отмена") { confirmingDelete = false }.buttonStyle(OutlineButton())
                Button("Удалить") {
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
                    Text("часы").font(.system(size: 10, weight: .medium))
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
            Image(systemName: StatusUI.icon(svc.status))
                .font(.system(size: 12))
                .foregroundStyle(StatusUI.color(svc.status))
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
                        Text("без проверки").font(.system(size: 10)).foregroundStyle(.secondary).fixedSize()
                    }
                    if model.isKeepOpen(svc.id) {
                        Image(systemName: "infinity").font(.system(size: 10, weight: .bold))
                            .foregroundStyle(Palette.accent).help("Держать открытым включён").fixedSize()
                    }
                }
                Text(StatusUI.line(svc, now: model.now))
                    .font(.system(size: 11)).foregroundStyle(svc.status == .open ? Palette.open : .secondary)
            }
            Spacer(minLength: 6)
            if svc.status == .knocking || svc.status == .checking {
                ProgressView().controlSize(.small)
            } else {
                Button { model.knock(svc) } label: { Image(systemName: "hand.tap.fill").font(.system(size: 13, weight: .semibold)) }
                    .buttonStyle(AccentButton()).help("Стукнуть и проверить")
                if svc.canCheck {
                    Button { model.check(svc) } label: { Image(systemName: "arrow.clockwise").font(.system(size: 13, weight: .semibold)) }
                        .buttonStyle(OutlineButton()).help("Только проверить порт, без стука")
                }
            }
            Button { model.openDetails(svc) } label: {
                Image(systemName: chevron).font(.system(size: 11)).foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("Детали и лог")
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
                    Text("Держать открытым").font(.system(size: 12.5, weight: .medium))
                }
                Text(can ? "Автоматически перестукивать незадолго до истечения доступа."
                         : "Недоступно: инвайт без таймаута — перевыпустите инвайт.")
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
            Text("ПОСЛЕДНИЕ СТУКИ")
                .font(.system(size: 10, weight: .semibold)).tracking(0.6).foregroundStyle(.secondary)
            if entries.isEmpty {
                Text("Пока нет попыток").font(.system(size: 11)).foregroundStyle(.secondary)
            } else {
                ForEach(entries) { e in
                    HStack(spacing: 8) {
                        Text(model.timeLabel(e.time))
                            .font(.system(size: 11, design: .monospaced)).foregroundStyle(.secondary)
                        Text(e.result.label)
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
                    Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 14, verticalSpacing: 5) {
                        detailRow("Роутер", svc.routerAddress, mono: true)
                        detailRow("Проверка", svc.checkPort > 0 ? "\(svc.checkPort)" : "check off", mono: true)
                        detailRow("Доступ на", model.ttlText(svc))
                        detailRow("Последний стук", model.lastKnockText(for: svc.id))
                    }
                    .padding(11)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(RoundedRectangle(cornerRadius: 9).fill(Color(.textBackgroundColor).opacity(0.5)))
                    .overlay(RoundedRectangle(cornerRadius: 9).stroke(Color.primary.opacity(0.08)))

                    HStack(spacing: 6) {
                        if svc.status == .knocking || svc.status == .checking {
                            ProgressView().controlSize(.small).frame(maxWidth: .infinity)
                        } else {
                            Button("Стук") { model.knock(svc) }.buttonStyle(AccentButton()).frame(maxWidth: .infinity)
                            if svc.canCheck {
                                Button("Проверить") { model.check(svc) }.buttonStyle(OutlineButton()).frame(maxWidth: .infinity)
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
                Text("Настройки").font(.system(size: 14.5, weight: .bold))
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 11)
            Divider()
            FittingScroll {
                VStack(alignment: .leading, spacing: 14) {
                    section("ОБЩЕЕ") {
                        card {
                            toggleRow("Автозапуск при входе",
                                      "Запускать mkpk при входе в систему.",
                                      isOn: $model.launchAtLogin)
                            Divider()
                            toggleRow("Уведомления",
                                      "Сообщать, когда доступ открылся или закрылся.",
                                      isOn: $model.notificationsEnabled)
                        }
                    }

                    section("СИНХРОНИЗАЦИЯ") {
                        card {
                            toggleRow("iCloud",
                                      "Синхронизировать инвайты через iCloud Keychain. Реальная синхронизация — в подписанной сборке.",
                                      isOn: $model.iCloudSync)
                        }
                    }

                    section("ДЕТАЛИ СЕРВИСА") {
                        card {
                            VStack(alignment: .leading, spacing: 8) {
                                Picker("", selection: $model.detailVariant) {
                                    Text("Инлайн").tag(AppModel.DetailVariant.inline)
                                    Text("Экран").tag(AppModel.DetailVariant.screen)
                                }
                                .pickerStyle(.segmented).labelsHidden()
                                Text("Как открывать детали и лог сервиса: раскрытием строки на месте или отдельным экраном.")
                                    .font(.system(size: 10.5)).foregroundStyle(.secondary)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                        }
                    }

                    HStack(spacing: 8) {
                        Brand.logo(.dark)
                            .resizable().interpolation(.high).frame(width: 16, height: 16)
                        Text(Self.aboutLine).font(.system(size: 11)).foregroundStyle(.secondary)
                        Spacer()
                        Button("Выйти") { model.quit() }
                            .buttonStyle(OutlineButton())
                            .help("Завершить mkpk (или правый клик по иконке)")
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
