import Cocoa
import CoreGraphics

guard CommandLine.arguments.count == 3,
      let amount = Int32(CommandLine.arguments[2]) else {
    print("Usage: scroll <direction> <amount>")
    exit(1)
}

let direction = CommandLine.arguments[1]

print("Scrolling \(direction) by amount: \(amount)")

let scrollAmount: Int32
switch direction {
case "up":
    scrollAmount = amount
case "down":
    scrollAmount = -amount
default:
    print("Invalid direction: \(direction). Use 'up' or 'down'")
    exit(1)
}

guard let mouseLocation = CGEvent(source: nil)?.location else {
    print("Failed to get mouse location")
    exit(1)
}

let scrollEvent = CGEvent(scrollWheelEvent2Source: nil,
                          units: .line,
                          wheelCount: 1,
                          wheel1: scrollAmount,
                          wheel2: 0,
                          wheel3: 0)
scrollEvent?.location = mouseLocation
scrollEvent?.post(tap: .cghidEventTap)

print("Scroll completed!")
