package qamcp

import (
	"context"
	"strconv"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// rescaleToScreen converts (x, y) from the model's view space (max_x × max_y)
// into the real screen coordinate space cached by the last screenshot. When
// no screenshot has been captured yet, the cache returns defaultResolution.
//
// On invalid input it returns an error result the caller should pass straight
// back to the MCP client.
func rescaleToScreen(x, y, maxX, maxY int) (int, int, *mcplib.CallToolResult) {
	if maxX <= 0 || maxY <= 0 {
		return 0, 0, mcplib.NewToolResultError("max_x and max_y must be positive")
	}
	res := GetScreenResolution()
	return x * res.Width / maxX, y * res.Height / maxY, nil
}

func clickHandler(ctx context.Context, request mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	x := request.GetInt("x", 0)
	y := request.GetInt("y", 0)
	maxX := request.GetInt("max_x", 0)
	maxY := request.GetInt("max_y", 0)

	realX, realY, errRes := rescaleToScreen(x, y, maxX, maxY)
	if errRes != nil {
		return errRes, nil
	}

	button := request.GetString("button", "left")
	dc := "false"
	if v, ok := request.GetArguments()["double_click"].(bool); ok && v {
		dc = "true"
	}

	out, err := clickBin.run(ctx,
		strconv.Itoa(realX),
		strconv.Itoa(realY),
		button,
		dc,
	)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("click", err), nil
	}
	return mcplib.NewToolResultText(out), nil
}

func typeHandler(ctx context.Context, request mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	text, err := request.RequireString("text")
	if err != nil {
		return mcplib.NewToolResultError(err.Error()), nil
	}
	out, err := typeBin.run(ctx, text)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("type", err), nil
	}
	return mcplib.NewToolResultText(out), nil
}

func scrollHandler(ctx context.Context, request mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	direction := request.GetString("direction", "down")
	amount := request.GetInt("amount", 3)

	out, err := scrollBin.run(ctx, direction, strconv.Itoa(amount))
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("scroll", err), nil
	}
	return mcplib.NewToolResultText(out), nil
}

func mouseDragHandler(ctx context.Context, request mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	startX := request.GetInt("start_x", 0)
	startY := request.GetInt("start_y", 0)
	endX := request.GetInt("end_x", 0)
	endY := request.GetInt("end_y", 0)
	maxX := request.GetInt("max_x", 0)
	maxY := request.GetInt("max_y", 0)

	rsx, rsy, errRes := rescaleToScreen(startX, startY, maxX, maxY)
	if errRes != nil {
		return errRes, nil
	}
	rex, rey, errRes := rescaleToScreen(endX, endY, maxX, maxY)
	if errRes != nil {
		return errRes, nil
	}

	out, err := mouseDragBin.run(ctx,
		strconv.Itoa(rsx),
		strconv.Itoa(rsy),
		strconv.Itoa(rex),
		strconv.Itoa(rey),
	)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("mouse drag", err), nil
	}
	return mcplib.NewToolResultText(out), nil
}
