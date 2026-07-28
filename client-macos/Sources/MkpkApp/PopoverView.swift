import SwiftUI
import MkpkKit

// Palette from the mockup (mkpk client.dc.html).
enum Palette {
    static let accent = Color(hex: 0x4753C5)
    static let open = Color(hex: 0x34C77B)
    static let warn = Color(hex: 0xF2A33C)
    static let error = Color(hex: 0xF26257)
    static let idle = Color(hex: 0x9A9AA2)
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

struct PopoverView: View {
    @ObservedObject var model: AppModel
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    ForEach(model.groups) { group in
                        RouterGroupView(model: model, group: group)
                    }
                }
                .padding(12)
            }
            Divider()
            Text("Стук открывает доступ только с вашего текущего IP")
                .font(.system(size: 11))
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 14).padding(.vertical, 9)
        }
        .frame(width: 380)
        .frame(minHeight: 200, maxHeight: 640)
        .background(.regularMaterial)
    }

    private var header: some View {
        HStack(spacing: 9) {
            Brand.logo(colorScheme)
                .resizable().interpolation(.high)
                .frame(width: 30, height: 30)
                .shadow(color: .black.opacity(colorScheme == .dark ? 0.55 : 0.22), radius: 3, x: 0, y: 1)
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 5) {
                    Text("mkpk").font(.system(size: 13, weight: .semibold))
                    Text("· Knock first").font(.system(size: 11)).foregroundStyle(.secondary)
                }
                Text("client_id: \(model.clientID)").font(.system(size: 11, design: .monospaced)).foregroundStyle(.secondary)
            }
            Spacer()
            Button { /* import — next */ } label: { Image(systemName: "plus") }.buttonStyle(.borderless)
            Button { /* settings — next */ } label: { Image(systemName: "gearshape") }.buttonStyle(.borderless)
        }
        .padding(.horizontal, 14).padding(.vertical, 11)
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
                Button { /* remove — next */ } label: { Image(systemName: "trash").font(.system(size: 11)) }.buttonStyle(.borderless).foregroundStyle(.secondary)
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

    var body: some View {
        HStack(spacing: 10) {
            Circle().fill(statusColor).frame(width: 8, height: 8)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(svc.name).font(.system(size: 13, weight: .semibold, design: .monospaced))
                    Text(svc.addressLabel).font(.system(size: 11, design: .monospaced)).foregroundStyle(.secondary).lineLimit(1)
                }
                Text(statusText).font(.system(size: 11)).foregroundStyle(.secondary)
            }
            Spacer(minLength: 6)
            if svc.status == .knocking || svc.status == .checking {
                ProgressView().controlSize(.small)
            } else {
                Button("Стук") { model.knock(svc) }.buttonStyle(AccentButton())
                if svc.canCheck {
                    Button("Проверить") { model.check(svc) }.buttonStyle(OutlineButton())
                }
            }
        }
        .padding(10)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color(.textBackgroundColor).opacity(0.5)))
        .overlay(RoundedRectangle(cornerRadius: 9).stroke(Color.primary.opacity(0.08)))
    }

    private var statusColor: Color {
        switch svc.status {
        case .open: return Palette.open
        case .closed: return Color(hex: 0x83838A)
        case .error: return Palette.error
        case .knocking, .checking: return Palette.accent
        case .unknown: return Palette.idle.opacity(0.5)
        }
    }
    private var statusText: String {
        switch svc.status {
        case .unknown: return "Не проверялось"
        case .checking: return "Проверяем…"
        case .knocking: return "Стучимся…"
        case .open: return "Открыто"
        case .closed: return "Закрыто"
        case .error: return "Ошибка"
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
