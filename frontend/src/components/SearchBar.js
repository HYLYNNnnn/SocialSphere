import React, { useState } from "react";
import { Input, Radio } from "antd";
import { SEARCH_KEY } from "../constants";

export default function SearchBar({ handleSearch }) {
  const [searchType, setSearchType] = useState(SEARCH_KEY.all);
  const [keyword, setKeyword] = useState("");
  const [error, setError] = useState("");
  const changeType = (event) => {
    const type = event.target.value;
    setSearchType(type); setError("");
    if (type === SEARCH_KEY.all || type === SEARCH_KEY.mine) handleSearch({ type, keyword: "" });
  };
  const search = (value) => {
    if (!value.trim() && searchType !== SEARCH_KEY.all && searchType !== SEARCH_KEY.mine) {
      setError("Enter a search keyword."); return;
    }
    setError(""); handleSearch({ type: searchType, keyword: value.trim() });
  };
  return (
    <div className="search-bar">
      <Input.Search placeholder="Search posts" aria-label="Search posts" enterButton="Search" size="large" onSearch={search}
        value={keyword} onChange={(event) => setKeyword(event.target.value)} disabled={searchType === SEARCH_KEY.all || searchType === SEARCH_KEY.mine} />
      <p className="error-msg" role={error ? "alert" : undefined}>{error}</p>
      <Radio.Group onChange={changeType} value={searchType} className="search-type-group">
        <Radio value={SEARCH_KEY.all}>All</Radio><Radio value={SEARCH_KEY.keyword}>Keyword</Radio>
        <Radio value={SEARCH_KEY.user}>User</Radio><Radio value={SEARCH_KEY.mine}>My posts</Radio>
      </Radio.Group>
    </div>
  );
}
