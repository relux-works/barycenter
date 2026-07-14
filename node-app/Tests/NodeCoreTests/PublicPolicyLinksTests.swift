import Testing

@testable import NodeCore

struct PublicPolicyLinksTests {
    @Test func linksUseStableUnauthenticatedHTTPSRoutes() {
        #expect(PublicPolicyLinks.all.count == 5)
        #expect(Set(PublicPolicyLinks.all.map(\.absoluteString)).count == 5)
        for url in PublicPolicyLinks.all {
            #expect(url.scheme == "https")
            #expect(url.host == "barycenter.live")
            #expect(url.user == nil)
            #expect(url.password == nil)
            #expect(url.query == nil)
            #expect(url.fragment == nil)
            #expect(url.path.hasPrefix("/legal/"))
            #expect(url.path.hasSuffix("/ru"))
        }
    }
}
