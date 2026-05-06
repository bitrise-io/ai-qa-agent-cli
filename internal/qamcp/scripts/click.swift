import Cocoa
import CoreGraphics

guard CommandLine.arguments.count == 5,
      let x = Double(CommandLine.arguments[1]),
      let y = Double(CommandLine.arguments[2]) else {
    print("Usage: click <x> <y> <button> <double_click>")
    exit(1)
}

let button = CommandLine.arguments[3]
let doubleClick = CommandLine.arguments[4] == "true"

let (mouseButton, mouseDownType, mouseUpType): (CGMouseButton, CGEventType, CGEventType)
switch button {
case "right":
    mouseButton = .right
    mouseDownType = .rightMouseDown
    mouseUpType = .rightMouseUp
case "middle":
    mouseButton = .center
    mouseDownType = .otherMouseDown
    mouseUpType = .otherMouseUp
default:
    mouseButton = .left
    mouseDownType = .leftMouseDown
    mouseUpType = .leftMouseUp
}

let point = CGPoint(x: x, y: y)

print("Clicking at: \(point.x), \(point.y) with \(button) button, double: \(doubleClick)")

let clickCount = doubleClick ? 2 : 1
for i in 1...clickCount {
    let mouseDown = CGEvent(mouseEventSource: nil, mouseType: mouseDownType, mouseCursorPosition: point, mouseButton: mouseButton)
    mouseDown?.setIntegerValueField(.mouseEventClickState, value: Int64(i))
    mouseDown?.post(tap: .cghidEventTap)

    usleep(50000)

    let mouseUp = CGEvent(mouseEventSource: nil, mouseType: mouseUpType, mouseCursorPosition: point, mouseButton: mouseButton)
    mouseUp?.setIntegerValueField(.mouseEventClickState, value: Int64(i))
    mouseUp?.post(tap: .cghidEventTap)

    if i < clickCount {
        usleep(50000)
    }
}

print("Click completed!")
