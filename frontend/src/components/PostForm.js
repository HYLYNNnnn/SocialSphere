import React, { forwardRef } from "react";
import { Form, Upload, Input, message } from "antd";
import { InboxOutlined } from "@ant-design/icons";
import { ACCEPTED_MEDIA_TYPES } from "../constants";
import { validateMediaFile } from "../api";

export const PostForm = forwardRef((props, formRef) => (
  <Form name="create_post" layout="vertical" ref={formRef}>
    <Form.Item name="description" label="Message" rules={[
      { required: true, whitespace: true, message: "Enter a message." }, { max: 2000, message: "Use at most 2000 characters." },
    ]}><Input.TextArea rows={3} maxLength={2000} showCount /></Form.Item>
    <Form.Item name="uploadPost" label="Image or video" valuePropName="fileList"
      getValueFromEvent={(event) => Array.isArray(event) ? event : event?.fileList}
      rules={[{ required: true, type: "array", min: 1, message: "Select an image or video." }]}>
      <Upload.Dragger name="files" maxCount={1} accept={ACCEPTED_MEDIA_TYPES.join(",")} beforeUpload={(file) => {
        try { validateMediaFile(file); return false; } catch (error) { message.error(error.message); return Upload.LIST_IGNORE; }
      }}>
        <p className="ant-upload-drag-icon"><InboxOutlined /></p>
        <p className="ant-upload-text">Click or drag one image or video here (maximum 10 MB).</p>
      </Upload.Dragger>
    </Form.Item>
  </Form>
));
