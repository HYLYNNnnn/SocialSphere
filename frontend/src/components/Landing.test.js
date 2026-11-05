import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import Landing from "./Landing";
import { generateImage, generatedImageFile, uploadPost } from "../api";
import { message } from "antd";

jest.mock("../api", () => ({ generateImage: jest.fn(), generatedImageFile: jest.fn(), uploadPost: jest.fn(), MAX_IMAGE_PROMPT_CHARACTERS: 2000 }));
jest.mock("antd", () => ({ message: { success: jest.fn(), error: jest.fn() } }));
jest.mock("styled-components", () => {
  const React = require("react");
  return { __esModule: true, default: { div: () => function Container({ children, ...props }) { return React.createElement("div", props, children); } } };
});
jest.mock("@mui/material", () => {
  const React = require("react");
  return {
    Typography: ({ children }) => React.createElement("span", null, children),
    Paper: ({ component: Tag = "div", children, onSubmit }) => React.createElement(Tag, { onSubmit }, children),
    InputBase: ({ inputProps, value, onChange, disabled }) => React.createElement("textarea", { ...inputProps, value, onChange, disabled }),
    IconButton: ({ children, onClick, type, disabled, "aria-label": label }) => React.createElement("button", { onClick, type, disabled, "aria-label": label }, children),
    CircularProgress: () => React.createElement("span", { role: "progressbar" }),
  };
});
jest.mock("@mui/icons-material/ArrowForward", () => () => null);
jest.mock("@mui/icons-material/FileUploadRounded", () => () => null);
jest.mock("react-photo-album", () => {
  const React = require("react");
  return function Album({ photos, onClick }) { return React.createElement("button", { disabled: !photos.length, onClick: () => onClick({ index: 0 }) }, "Preview image"); };
});
jest.mock("yet-another-react-lightbox", () => {
  const React = require("react");
  return function Lightbox({ open, toolbar }) { return open ? React.createElement("div", null, toolbar.buttons.filter((button) => typeof button !== "string")) : null; };
}, { virtual: true });
jest.mock("yet-another-react-lightbox/plugins/fullscreen", () => () => null, { virtual: true });
jest.mock("yet-another-react-lightbox/plugins/slideshow", () => () => null, { virtual: true });
jest.mock("yet-another-react-lightbox/plugins/thumbnails", () => () => null, { virtual: true });
jest.mock("yet-another-react-lightbox/plugins/zoom", () => () => null, { virtual: true });

const image = { imageUrl: "data:image/png;base64,cG5n", prompt: "A blue bird" };
beforeEach(() => {
  generateImage.mockResolvedValue(image);
  generatedImageFile.mockReturnValue(new File(["png"], "generated.png", { type: "image/png" }));
  uploadPost.mockResolvedValue({ id: "post-1" });
});

async function generate() {
  fireEvent.change(screen.getByRole("textbox", { name: "Image description" }), { target: { value: image.prompt } });
  fireEvent.click(screen.getByRole("button", { name: "Generate image" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Preview image" })).toBeEnabled());
}

test("generation requires a description and backend failure resets loading", async () => {
  generateImage.mockRejectedValue(new Error("AI is not configured"));
  render(<Landing />);
  expect(screen.getByRole("button", { name: "Generate image" })).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox", { name: "Image description" }), { target: { value: "A bird" } });
  fireEvent.click(screen.getByRole("button", { name: "Generate image" }));
  await waitFor(() => expect(message.error).toHaveBeenCalledWith("AI is not configured"));
  expect(screen.getByRole("button", { name: "Generate image" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Preview image" })).toBeDisabled();
});

test("publishes the generated PNG through the normal post API", async () => {
  render(<Landing />); await generate();
  expect(generateImage).toHaveBeenCalledWith("A blue bird");
  fireEvent.click(screen.getByRole("button", { name: "Preview image" }));
  fireEvent.click(screen.getByRole("button", { name: "Publish generated image" }));
  await waitFor(() => expect(uploadPost).toHaveBeenCalledTimes(1));
  expect(generatedImageFile).toHaveBeenCalledWith(image.imageUrl);
  expect(uploadPost.mock.calls[0][0].type).toBe("image/png");
  expect(uploadPost.mock.calls[0][1]).toBe(image.prompt);
  expect(message.success).toHaveBeenCalled();
});

test("a failed publish can be retried without losing the generated image", async () => {
  uploadPost.mockRejectedValueOnce(new Error("Upload unavailable")).mockResolvedValueOnce({ id: "post-1" });
  render(<Landing />); await generate();
  fireEvent.click(screen.getByRole("button", { name: "Preview image" }));
  fireEvent.click(screen.getByRole("button", { name: "Publish generated image" }));
  await waitFor(() => expect(message.error).toHaveBeenCalledWith("Upload unavailable"));
  expect(screen.getByRole("button", { name: "Publish generated image" })).toBeEnabled();
  fireEvent.click(screen.getByRole("button", { name: "Publish generated image" }));
  await waitFor(() => expect(uploadPost).toHaveBeenCalledTimes(2));
});


test("blocks 2001-character prompts but accepts 2000 emoji without UTF-16 truncation", async () => {
  render(<Landing />);
  const description = screen.getByRole("textbox", { name: "Image description" });
  fireEvent.change(description, { target: { value: "a".repeat(2001) } });
  expect(screen.getByRole("button", { name: "Generate image" })).toBeDisabled();
  expect(screen.getByText("2001 / 2000 characters — shorten your description.")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Generate image" }));
  expect(generateImage).not.toHaveBeenCalled();
  const prompt = "🦉".repeat(2000);
  fireEvent.change(description, { target: { value: prompt } });
  expect(description).toHaveValue(prompt);
  expect(screen.getByRole("button", { name: "Generate image" })).toBeEnabled();
  expect(screen.getByText("2000 / 2000 characters")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Generate image" }));
  await waitFor(() => expect(generateImage).toHaveBeenCalledWith(prompt));
});
