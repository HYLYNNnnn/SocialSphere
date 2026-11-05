import React, { useState } from "react";
import { Form, Input, Button, message } from "antd";
import { UserOutlined, LockOutlined } from "@ant-design/icons";
import { Link } from "react-router-dom";
import { signIn } from "../api";

export default function Login({ handleLoggedIn }) {
  const [submitting, setSubmitting] = useState(false);
  const onFinish = async (values) => {
    if (submitting) return;
    setSubmitting(true);
    try {
      const session = await signIn(values);
      handleLoggedIn(session);
      message.success("Signed in.");
    } catch (error) { message.error(error.message); }
    finally { setSubmitting(false); }
  };
  return (
    <Form name="normal_login" className="login-form" onFinish={onFinish}>
      <Form.Item name="username" rules={[{ required: true, whitespace: true, message: "Enter your username." }]}>
        <Input autoComplete="username" prefix={<UserOutlined />} placeholder="Username" disabled={submitting} />
      </Form.Item>
      <Form.Item name="password" rules={[{ required: true, message: "Enter your password." }]}>
        <Input.Password autoComplete="current-password" prefix={<LockOutlined />} placeholder="Password" disabled={submitting} />
      </Form.Item>
      <Form.Item>
        <Button type="primary" htmlType="submit" className="login-form-button" loading={submitting} style={{ backgroundColor: "black" }}>Log in</Button>
        Or <Link to="/register">register now!</Link>
      </Form.Item>
    </Form>
  );
}
