import Cocoa
import CoreGraphics

guard CommandLine.arguments.count == 2 else {
    print("Usage: type <text>")
    exit(1)
}

let text = CommandLine.arguments[1]

print("Typing text: \(text)")

for character in text {
    let string = String(character)

    if let keyDown = CGEvent(keyboardEventSource: nil, virtualKey: 0, keyDown: true) {
        var unicodeArray = Array(string.utf16)
        keyDown.keyboardSetUnicodeString(stringLength: unicodeArray.count, unicodeString: &unicodeArray)
        keyDown.post(tap: .cghidEventTap)
    }

    usleep(10000)

    if let keyUp = CGEvent(keyboardEventSource: nil, virtualKey: 0, keyDown: false) {
        var unicodeArray = Array(string.utf16)
        keyUp.keyboardSetUnicodeString(stringLength: unicodeArray.count, unicodeString: &unicodeArray)
        keyUp.post(tap: .cghidEventTap)
    }

    usleep(10000)
}

print("Typing completed!")
