import React, { useState, useEffect } from "react";
import { Tabs, Row, Col, Alert, Button, Spin, Empty } from "antd";
import SearchBar from "./SearchBar";
import { SEARCH_KEY } from "../constants";
import { searchPosts } from "../api";
import PhotoGallery from "./PhotoGallery";
import CreatePostButton from "./CreatePostButton";

export default function Collection({ username }) {
  const [posts, setPosts] = useState([]);
  const [activeTab, setActiveTab] = useState("image");
  const [searchOption, setSearchOption] = useState({ type: SEARCH_KEY.all, keyword: "" });
  const [reload, setReload] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError(""); setPosts([]);
    searchPosts(searchOption, { signal: controller.signal })
      .then((data) => { if (!controller.signal.aborted) setPosts(data); })
      .catch((failure) => { if (!controller.signal.aborted) setError(failure.message); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [searchOption, reload]);
  const showPost = (type) => {
    setActiveTab(type);
    setSearchOption({ type: SEARCH_KEY.all, keyword: "" });
    setReload((value) => value + 1);
  };
  const renderPosts = (type) => {
    const filtered = posts.filter((post) => post.type === type);
    if (!filtered.length) return <Empty description={type === "image" ? "No images found." : "No videos found."} />;
    if (type === "image") return <PhotoGallery username={username} images={filtered.map((post) => ({
      postId: post.id, src: post.url, user: post.user, caption: post.message,
    }))} onDeleted={(id) => setPosts((current) => current.filter((post) => post.id !== id))} />;
    return <Row>{filtered.map((post) => <Col span={24} key={post.id}><p>{post.user}: {post.message}</p><video src={post.url} controls className="video-block" /></Col>)}</Row>;
  };
  return (
    <div className="home">
      <SearchBar handleSearch={setSearchOption} />
      {error ? <Alert type="error" showIcon message="Unable to load posts" description={error}
        action={<Button onClick={() => setReload((value) => value + 1)}>Retry</Button>} /> : <Spin spinning={loading}>
        <div className="display"><Tabs activeKey={activeTab} onChange={setActiveTab}
          tabBarExtraContent={<CreatePostButton onShowPost={showPost} />}
          items={[{ key: "image", label: "Images", children: renderPosts("image") }, { key: "video", label: "Videos", children: renderPosts("video") }]} /></div>
      </Spin>}
    </div>
  );
}
