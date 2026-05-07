import SwiftUI

@main
struct HelloApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
        }
    }
}

struct ContentView: View {
    @State private var taps = 0

    var body: some View {
        VStack(spacing: 24) {
            Text("Hello, RDE!")
                .font(.largeTitle)
            Text("Taps: \(taps)")
                .font(.title2)
                .accessibilityIdentifier("tap-count")
            Button("Tap me") {
                taps += 1
            }
            .buttonStyle(.borderedProminent)
            .accessibilityIdentifier("tap-button")

            if taps >= 5 {
                Text("High five!")
                    .font(.title3)
                    .accessibilityIdentifier("high-five")
            }
        }
        .padding()
    }
}
