import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Form, Input, Button, message } from "antd";
import { signUp, validatePassword } from "../api";

export default function Register() {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();
  const onFinish = async (values) => {
    if (submitting) return;
    setSubmitting(true);
    try {
      await signUp(values);
      message.success("Registration succeeded. Sign in with your new account.");
      navigate("/login", { replace: true });
    } catch (error) { message.error(error.message); }
    finally { setSubmitting(false); }
  };
  return (
    <Form form={form} name="register" onFinish={onFinish} className="register" labelCol={{ xs: { span: 24 }, sm: { span: 8 } }} wrapperCol={{ xs: { span: 24 }, sm: { span: 16 } }}>
      <Form.Item name="username" label="Username" rules={[
        { required: true, whitespace: true, message: "Enter a username." },
        { pattern: /^[A-Za-z0-9][A-Za-z0-9_]{2,31}$/, message: "Use 3–32 letters, numbers or underscores; start with a letter or number." },
      ]}><Input autoComplete="username" disabled={submitting} /></Form.Item>
      <Form.Item name="password" label="Password" hasFeedback rules={[
        { required: true, message: "Enter a password." },
        { validator: (_, value) => {
          if (!value) return Promise.resolve();
          try { validatePassword(value); return Promise.resolve(); }
          catch (error) { return Promise.reject(error); }
        } },
      ]}><Input.Password autoComplete="new-password" disabled={submitting} /></Form.Item>
      <Form.Item name="confirm" label="Confirm Password" dependencies={["password"]} hasFeedback rules={[
        { required: true, message: "Confirm your password." },
        ({ getFieldValue }) => ({ validator: (_, value) => !value || getFieldValue("password") === value
          ? Promise.resolve() : Promise.reject(new Error("The passwords do not match.")) }),
      ]}><Input.Password autoComplete="new-password" disabled={submitting} /></Form.Item>
      <Form.Item wrapperCol={{ xs: { span: 24 }, sm: { span: 16, offset: 8 } }}>
        <Button type="primary" htmlType="submit" className="register-btn" loading={submitting} style={{ backgroundColor: "black" }}>Register</Button>
      </Form.Item>
    </Form>
  );
}
