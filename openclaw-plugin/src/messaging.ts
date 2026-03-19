import type { LinkClawPluginConfig } from "./bridge.ts";
import { runLinkClaw } from "./bridge.ts";
import { runSyncCommand } from "./commands.ts";

type InboxConversation = {
  contact_display_name?: string;
  contact_canonical_id?: string;
  contact_status?: string;
  unread_count?: number;
  last_message_preview?: string;
};

type InboxResult = {
  conversations?: InboxConversation[];
};

export async function triggerBackgroundSync(
  config: LinkClawPluginConfig,
  pluginRoot: string,
): Promise<string | undefined> {
  try {
    const result = await runSyncCommand(config, "", pluginRoot);
    const match = result.message.match(/synced: (\d+)/);
    if (!match) {
      return undefined;
    }
    const synced = Number(match[1]);
    if (!Number.isFinite(synced) || synced <= 0) {
      return undefined;
    }
    const inboxEnvelope = await runLinkClaw(
      config,
      {
        command: "message_inbox",
      },
      pluginRoot,
    );
    return formatBackgroundSyncNotice(result.message, inboxEnvelope.result as InboxResult);
  } catch {
    return undefined;
  }
}

export function appendSyncMessage(event: unknown, message: string): boolean {
  if (typeof event !== "object" || event === null || message.trim() === "") {
    return false;
  }
  const record = event as Record<string, unknown>;
  const messages = Array.isArray(record.messages) ? record.messages : [];
  if (!Array.isArray(record.messages)) {
    record.messages = messages;
  }
  messages.push({
    role: "assistant",
    content: message,
  });
  return true;
}

export function formatBackgroundSyncNotice(syncMessage: string, inbox: InboxResult): string {
  const unknownSenders = extractUnknownSenders(inbox);
  if (unknownSenders.length === 0) {
    return syncMessage;
  }

  const lines = [syncMessage, "", "LinkClaw found messages from unknown senders:"];
  for (const sender of unknownSenders.slice(0, 3)) {
    const name = sender.contact_display_name?.trim() || "(unknown)";
    const canonicalID = sender.contact_canonical_id?.trim() || "";
    const preview = sender.last_message_preview?.trim() || "";
    const unread = Number.isFinite(sender.unread_count) ? sender.unread_count : 0;
    const summary = [name];
    if (canonicalID !== "") {
      summary.push(canonicalID);
    }
    summary.push(`unread=${unread}`);
    if (preview !== "") {
      summary.push(`last=${preview}`);
    }
    lines.push(`- ${summary.join(" | ")}`);
  }
  lines.push("Run /linkclaw-inbox to review them.");
  lines.push("If you want to keep one, ask for an identity card and run /linkclaw-connect <card>.");
  return lines.join("\n");
}

export function extractUnknownSenders(inbox: InboxResult): InboxConversation[] {
  const conversations = Array.isArray(inbox?.conversations) ? inbox.conversations : [];
  return conversations.filter(
    (conversation) =>
      typeof conversation === "object" &&
      conversation !== null &&
      conversation.contact_status === "discovered" &&
      Number(conversation.unread_count ?? 0) > 0,
  );
}
