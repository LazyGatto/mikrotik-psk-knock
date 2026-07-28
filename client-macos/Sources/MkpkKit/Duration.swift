import Foundation

/// Parses a Go-style duration string (as carried in the invite's allowed_timeout,
/// e.g. "3m", "90s", "1h30m", "500ms") into seconds. Returns nil if unparseable.
public enum GoDuration {
    public static func seconds(_ s: String) -> TimeInterval? {
        let str = s.trimmingCharacters(in: .whitespaces)
        guard !str.isEmpty else { return nil }
        var total: Double = 0
        var idx = str.startIndex
        var parsedAny = false
        while idx < str.endIndex {
            var number = ""
            while idx < str.endIndex, str[idx].isNumber || str[idx] == "." {
                number.append(str[idx]); idx = str.index(after: idx)
            }
            guard let value = Double(number) else { return nil }
            var unit = ""
            while idx < str.endIndex, !(str[idx].isNumber || str[idx] == ".") {
                unit.append(str[idx]); idx = str.index(after: idx)
            }
            let multiplier: Double
            switch unit {
            case "ns": multiplier = 1e-9
            case "us", "µs": multiplier = 1e-6
            case "ms": multiplier = 1e-3
            case "s": multiplier = 1
            case "m": multiplier = 60
            case "h": multiplier = 3600
            default: return nil
            }
            total += value * multiplier
            parsedAny = true
        }
        return parsedAny ? total : nil
    }
}
