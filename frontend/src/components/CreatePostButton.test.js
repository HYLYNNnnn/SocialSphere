import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import CreatePostButton from "./CreatePostButton";
import { uploadPost } from "../api";
import { message } from "antd";

const mockValidate = jest.fn();
const mockReset = jest.fn();
jest.mock("../api", () => ({ ...jest.requireActual("../api"), uploadPost: jest.fn() }));
jest.mock("./PostForm", () => {
  const React = require("react");
  return { PostForm: React.forwardRef(function Form(props, ref) {
    React.useImperativeHandle(ref, () => ({ validateFields: mockValidate, resetFields: mockReset }));
    return React.createElement("div", null, "Post form");
  }) };
});
jest.mock("antd", () => {
  const React = require("react");
  return {
    Button: ({ children, onClick }) => React.createElement("button", { onClick }, children),
    Modal: ({ open, onOk, onCancel, confirmLoading, children }) => open ? React.createElement("div", { role: "dialog" }, children,
      React.createElement("button", { onClick: onOk, disabled: confirmLoading }, "Create"),
      React.createElement("button", { onClick: onCancel }, "Cancel")) : null,
    message: { error: jest.fn(), success: jest.fn() },
  };
});

beforeEach(() => {
  mockValidate.mockResolvedValue({ description: "A new post", uploadPost: [{ originFileObj: new File(["png"], "photo.png", { type: "image/png" }) }] });
  uploadPost.mockResolvedValue({ id: "post-1" });
});
const open = () => fireEvent.click(screen.getByRole("button", { name: "Create New Post" }));

test("validation failure resets the submit state instead of leaving a spinner", async () => {
  mockValidate.mockRejectedValue({ errorFields: [{ name: ["description"] }] });
  render(<CreatePostButton onShowPost={jest.fn()} />); open();
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Create" })).toBeEnabled());
  expect(uploadPost).not.toHaveBeenCalled();
});

test("a missing media file shows an error and permits correction", async () => {
  mockValidate.mockResolvedValue({ description: "A new post", uploadPost: [] });
  render(<CreatePostButton onShowPost={jest.fn()} />); open();
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => expect(message.error).toHaveBeenCalledWith("Please select an image or video."));
  expect(screen.getByRole("button", { name: "Create" })).toBeEnabled();
  expect(uploadPost).not.toHaveBeenCalled();
});

test("a successful upload immediately refreshes the appropriate media tab", async () => {
  const refresh = jest.fn();
  render(<CreatePostButton onShowPost={refresh} />); open();
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => expect(refresh).toHaveBeenCalledWith("image"));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(mockReset).toHaveBeenCalledTimes(1);
});
