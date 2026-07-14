import Foundation

public enum PublicPolicyLinks {
    public static let privacy = URL(string: "https://barycenter.live/legal/privacy/ru")!
    public static let terms = URL(string: "https://barycenter.live/legal/terms/ru")!
    public static let contentGuidelines = URL(string: "https://barycenter.live/legal/content-guidelines/ru")!
    public static let uploadRights = URL(string: "https://barycenter.live/legal/upload-rights/ru")!
    public static let support = URL(string: "https://barycenter.live/legal/support/ru")!

    public static let all: [URL] = [privacy, terms, contentGuidelines, uploadRights, support]
}
