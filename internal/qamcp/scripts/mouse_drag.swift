import Cocoa
import CoreGraphics

guard CommandLine.arguments.count == 5,
      let startX = Double(CommandLine.arguments[1]),
      let startY = Double(CommandLine.arguments[2]),
      let endX = Double(CommandLine.arguments[3]),
      let endY = Double(CommandLine.arguments[4]) else {
    print("Usage: mouse_drag <start_x> <start_y> <end_x> <end_y>")
    exit(1)
}

let startPoint = CGPoint(x: startX, y: startY)
let endPoint = CGPoint(x: endX, y: endY)

print("Dragging from: \(startPoint.x), \(startPoint.y) to: \(endPoint.x), \(endPoint.y)")

let mouseDown = CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: startPoint, mouseButton: .left)
mouseDown?.post(tap: .cghidEventTap)

usleep(50000)

let distance = sqrt((endX - startX) * (endX - startX) + (endY - startY) * (endY - startY))
let steps = max(10, Int(distance / 2))
for i in 0...steps {
    let progress = Double(i) / Double(steps)
    let currentX = startX + (endX - startX) * progress
    let currentY = startY + (endY - startY) * progress
    let currentPoint = CGPoint(x: currentX, y: currentY)

    let mouseDragged = CGEvent(mouseEventSource: nil, mouseType: .leftMouseDragged, mouseCursorPosition: currentPoint, mouseButton: .left)
    mouseDragged?.post(tap: .cghidEventTap)

    usleep(500)
}

let mouseUp = CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: endPoint, mouseButton: .left)
mouseUp?.post(tap: .cghidEventTap)

print("Drag completed!")
