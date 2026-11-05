import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import Landing from "./Landing";
import { getImageConfig, generateImage, generatedImageFile, uploadPost } from "../api";
import { message } from "antd";

jest.mock("../api", () => ({ getImageConfig: jest.fn(), generateImage: jest.fn(), generatedImageFile: jest.fn(), uploadPost: jest.fn(), MAX_IMAGE_PROMPT_CHARACTERS: 2000 }));
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
const imageConfig = { server_configured: true, user_keys_allowed: true, default_model: "gpt-image-2.6-flare",
  models: [{ id: "gpt-image-2", label: "GPT Image 2" }, { id: "gpt-image-2.6-flare", label: "GPT Image 2.6 Flare" }] };
beforeEach(() => {
  localStorage.clear(); sessionStorage.clear();
  getImageConfig.mockResolvedValue(imageConfig);
  generateImage.mockResolvedValue(image);
  generatedImageFile.mockReturnValue(new File(["png"], "generated.png", { type: "image/png" }));
  uploadPost.mockResolvedValue({ id: "post-1" });
});

async function generate() {
  await screen.findByRole("combobox", { name: "Image model" });
  fireEvent.change(screen.getByRole("textbox", { name: "Image description" }), { target: { value: image.prompt } });
  fireEvent.click(screen.getByRole("button", { name: "Generate image" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Preview image" })).toBeEnabled());
}

test("generation requires a description and backend failure resets loading", async () => {
  generateImage.mockRejectedValue(new Error("AI is not configured"));
  render(<Landing />);
  await screen.findByRole("combobox", { name: "Image model" });
  expect(screen.getByRole("button", { name: "Generate image" })).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox", { name: "Image description" }), { target: { value: "A bird" } });
  fireEvent.click(screen.getByRole("button", { name: "Generate image" }));
  await waitFor(() => expect(message.error).toHaveBeenCalledWith("AI is not configured"));
  expect(screen.getByRole("button", { name: "Generate image" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Preview image" })).toBeDisabled();
});

test("publishes the generated PNG through the normal post API", async () => {
  render(<Landing />); await generate();
  expect(generateImage).toHaveBeenCalledWith("A blue bird", { model: "gpt-image-2.6-flare", keySource: "server" });
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
  await screen.findByRole("combobox", { name: "Image model" });
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
  await waitFor(() => expect(generateImage).toHaveBeenCalledWith(prompt, { model: "gpt-image-2.6-flare", keySource: "server" }));
});

test("personal keys stay in page memory, are sent only on Generate, and can be cleared", async () => {
  const cookieBefore = document.cookie;
  render(<Landing />);
  const source = await screen.findByRole("combobox", { name: "API key source" });
  expect(screen.getByRole("combobox", { name: "Image model" })).toHaveValue("gpt-image-2.6-flare");
  fireEvent.change(source, { target: { value: "personal" } });
  expect(screen.getByRole("combobox", { name: "Image model" })).toHaveValue("gpt-image-2");
  const key = screen.getByLabelText("OpenAI API key");
  expect(key).toHaveAttribute("type", "password");
  fireEvent.change(screen.getByRole("textbox", { name: "Image description" }), { target: { value: image.prompt } });
  expect(screen.getByRole("button", { name: "Generate image" })).toBeDisabled();
  fireEvent.change(key, { target: { value: "sk-example-personal-key" } });
  expect(localStorage.length).toBe(0); expect(sessionStorage.length).toBe(0); expect(document.cookie).toBe(cookieBefore);
  expect(generateImage).not.toHaveBeenCalled();
  fireEvent.change(screen.getByRole("combobox", { name: "Image model" }), { target: { value: "gpt-image-2.6-flare" } });
  fireEvent.click(screen.getByRole("button", { name: "Generate image" }));
  await waitFor(() => expect(generateImage).toHaveBeenCalledWith(image.prompt,
    { model: "gpt-image-2.6-flare", keySource: "personal", apiKey: "sk-example-personal-key" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Clear key" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Clear key" }));
  expect(key).toHaveValue(""); expect(screen.getByRole("button", { name: "Generate image" })).toBeDisabled();
});

test("leaving the creation page discards the personal key and cancels its settings load", async () => {
  getImageConfig.mockResolvedValue({ ...imageConfig, server_configured: false });
  const page = render(<Landing />);
  const key = await screen.findByLabelText("OpenAI API key");
  fireEvent.change(key, { target: { value: "sk-example-not-saved" } });
  const signal = getImageConfig.mock.calls[0][0].signal;
  page.unmount();
  expect(signal.aborted).toBe(true);
  render(<Landing />);
  expect(await screen.findByLabelText("OpenAI API key")).toHaveValue("");
  expect(localStorage.length).toBe(0); expect(sessionStorage.length).toBe(0);
  expect(generateImage).not.toHaveBeenCalled();
});

test("switching to the site key clears the personal key and restores the site model", async () => {
  render(<Landing />);
  const source = await screen.findByRole("combobox", { name: "API key source" });
  fireEvent.change(source, { target: { value: "personal" } });
  fireEvent.change(screen.getByLabelText("OpenAI API key"), { target: { value: "sk-example-not-saved" } });
  fireEvent.change(source, { target: { value: "server" } });
  expect(screen.queryByLabelText("OpenAI API key")).not.toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "Image model" })).toHaveValue(imageConfig.default_model);
  fireEvent.change(source, { target: { value: "personal" } });
  expect(screen.getByLabelText("OpenAI API key")).toHaveValue("");
});

test("site availability controls the choices and generation stays blocked when disabled", async () => {
  getImageConfig.mockResolvedValue({ ...imageConfig, server_configured: false, user_keys_allowed: false });
  render(<Landing />);
  expect(await screen.findByText(/Image creation is not available/)).toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "API key source" })).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole("textbox", { name: "Image description" }), { target: { value: image.prompt } });
  expect(screen.getByRole("button", { name: "Generate image" })).toBeDisabled();
  expect(generateImage).not.toHaveBeenCalled();
});

test("settings failures offer a retry without sending a generation request", async () => {
  getImageConfig.mockRejectedValueOnce(new Error("Unavailable")).mockResolvedValueOnce({ ...imageConfig, user_keys_allowed: false });
  render(<Landing />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Image settings could not be loaded");
  expect(screen.getByRole("button", { name: "Generate image" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  expect(await screen.findByRole("combobox", { name: "API key source" })).toHaveValue("server");
  expect(screen.queryByRole("option", { name: "Use my API key" })).not.toBeInTheDocument();
  expect(generateImage).not.toHaveBeenCalled();
});
