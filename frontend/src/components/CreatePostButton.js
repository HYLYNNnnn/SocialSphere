import React, { useRef, useState } from "react";
import { Modal, Button, message } from "antd";
import { PostForm } from "./PostForm";
import { uploadPost, validateMediaFile } from "../api";

export default function CreatePostButton({ onShowPost }) {
  const form = useRef(null);
  const pending = useRef(false);
  const [visible, setVisible] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const submit = async () => {
    if (pending.current) return;
    pending.current = true; setSubmitting(true);
    try {
      const values = await form.current.validateFields();
      const file = values.uploadPost?.[0]?.originFileObj;
      const type = validateMediaFile(file);
      await uploadPost(file, values.description);
      form.current.resetFields(); setVisible(false);
      message.success("Your post was uploaded."); onShowPost(type);
    } catch (error) {
      if (!error.errorFields) message.error(error.message || "Unable to upload the post.");
    } finally { pending.current = false; setSubmitting(false); }
  };
  return <>
    <Button type="primary" onClick={() => setVisible(true)}>Create New Post</Button>
    <Modal title="Create New Post" open={visible} onOk={submit} okText="Create" confirmLoading={submitting}
      onCancel={() => { if (!submitting) setVisible(false); }} closable={!submitting} maskClosable={!submitting} cancelButtonProps={{ disabled: submitting }}>
      <PostForm ref={form} />
    </Modal>
  </>;
}
