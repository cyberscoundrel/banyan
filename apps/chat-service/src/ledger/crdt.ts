import { Entry, State, Post, Channel, Ban, User, EntryType, PostData, ChannelCreateData, BanData, PromoteData } from './types.js';

export function compareEntries(a: Entry, b: Entry): number {
  if (a.timestamp !== b.timestamp) {
    return b.timestamp - a.timestamp;
  }
  return a.author.localeCompare(b.author);
}

export function mergeEntries(local: Entry[], remote: Entry[]): Entry[] {
  const merged = new Map<string, Entry>();
  
  for (const entry of [...local, ...remote]) {
    const existing = merged.get(entry.id);
    if (!existing || compareEntries(entry, existing) > 0) {
      merged.set(entry.id, entry);
    }
  }
  
  return Array.from(merged.values());
}

export function mergeIntoState(state: State, entries: Entry[]): State {
  const newState: State = {
    entries: new Map(state.entries),
    posts: new Map(state.posts),
    channels: new Map(state.channels),
    bans: new Map(state.bans),
    users: new Map(state.users),
    latestHash: state.latestHash,
  };

  const sorted = [...entries].sort((a, b) => a.timestamp - b.timestamp);

  for (const entry of sorted) {
    const existing = newState.entries.get(entry.id);
    if (existing && existing.hash === entry.hash) continue;
    
    newState.entries.set(entry.id, entry);

    switch (entry.type) {
      case EntryType.Post: {
        const data = entry.data as PostData;
        const post: Post = {
          id: entry.id,
          content: data.content,
          channel: data.channel,
          author: entry.author,
          timestamp: entry.timestamp,
          replyTo: data.replyTo,
          deleted: false,
        };
        newState.posts.set(entry.id, post);
        break;
      }
      case EntryType.Delete: {
        const data = entry.data as { postId: string };
        const post = newState.posts.get(data.postId);
        if (post) {
          post.deleted = true;
        }
        break;
      }
      case EntryType.ChannelCreate: {
        const data = entry.data as ChannelCreateData;
        const channel: Channel = {
          name: data.name,
          description: data.description,
          createdAt: entry.timestamp,
          createdBy: entry.author,
        };
        newState.channels.set(data.name, channel);
        break;
      }
      case EntryType.ChannelDelete: {
        const data = entry.data as { name: string };
        newState.channels.delete(data.name);
        break;
      }
      case EntryType.Ban: {
        const data = entry.data as BanData;
        const ban: Ban = {
          peerId: data.peerId,
          reason: data.reason,
          bannedAt: entry.timestamp,
          bannedBy: entry.author,
        };
        newState.bans.set(data.peerId, ban);
        break;
      }
      case EntryType.Promote: {
        const data = entry.data as PromoteData;
        const user: User = {
          peerId: data.peerId,
          role: data.role,
          promotedAt: entry.timestamp,
          promotedBy: entry.author,
        };
        newState.users.set(data.peerId, user);
        break;
      }
    }

    if (entry.timestamp > (newState.entries.get(newState.latestHash)?.timestamp || 0)) {
      newState.latestHash = entry.hash;
    }
  }

  return newState;
}

export function getActivePosts(state: State): Post[] {
  return Array.from(state.posts.values())
    .filter(p => !p.deleted)
    .sort((a, b) => b.timestamp - a.timestamp);
}

export function getActiveChannels(state: State): Channel[] {
  return Array.from(state.channels.values())
    .sort((a, b) => a.name.localeCompare(b.name));
}

export function isBanned(state: State, peerId: string): boolean {
  return state.bans.has(peerId);
}

export function getUserRole(state: State, peerId: string): 'user' | 'moderator' | 'admin' {
  const user = state.users.get(peerId);
  return user?.role || 'user';
}
