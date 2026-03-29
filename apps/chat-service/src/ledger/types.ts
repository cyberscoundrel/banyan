export enum EntryType {
  Post = 'post',
  Delete = 'delete',
  ChannelCreate = 'channelCreate',
  ChannelDelete = 'channelDelete',
  Ban = 'ban',
  Promote = 'promote',
  IssueKey = 'issueKey',
  Revoke = 'revoke',
}

export interface PostData {
  content: string;
  channel: string;
  replyTo?: string;
}

export interface DeleteData {
  postId: string;
}

export interface ChannelCreateData {
  name: string;
  description?: string;
}

export interface ChannelDeleteData {
  name: string;
}

export interface BanData {
  peerId: string;
  reason?: string;
}

export interface PromoteData {
  peerId: string;
  role: 'moderator' | 'admin';
}

export interface IssueKeyData {
  peerId: string;
  keyType: 'static' | 'ledger' | 'mod' | 'admin';
}

export interface RevokeData {
  peerId: string;
  keyType?: string;
}

export type EntryData = PostData | DeleteData | ChannelCreateData | ChannelDeleteData | BanData | PromoteData | IssueKeyData | RevokeData;

export interface Entry<T extends EntryData = EntryData> {
  id: string;
  type: EntryType;
  author: string;
  timestamp: number;
  data: T;
  signature: string;
  hash: string;
  prevHash: string;
  deleted?: boolean;
}

export interface Channel {
  name: string;
  description?: string;
  createdAt: number;
  createdBy: string;
}

export interface Post {
  id: string;
  content: string;
  channel: string;
  author: string;
  timestamp: number;
  replyTo?: string;
  deleted: boolean;
}

export interface Ban {
  peerId: string;
  reason?: string;
  bannedAt: number;
  bannedBy: string;
}

export interface User {
  peerId: string;
  role: 'user' | 'moderator' | 'admin';
  promotedAt: number;
  promotedBy: string;
}

export interface State {
  entries: Map<string, Entry>;
  posts: Map<string, Post>;
  channels: Map<string, Channel>;
  bans: Map<string, Ban>;
  users: Map<string, User>;
  latestHash: string;
}
