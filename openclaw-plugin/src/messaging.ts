import type { LinkClawPluginConfig } from "./bridge.ts";
import { runLinkClaw } from "./bridge.ts";
import { runSyncCommand } from "./commands.ts";

const DEFAULT_BACKGROUND_SYNC_INTERVAL_MS = 30_000;

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

type PluginLogger = {
  info?: (message: string) => void;
  warn?: (message: string) => void;
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

export function createBackgroundSyncService(params: {
  config: LinkClawPluginConfig;
  pluginRoot: string;
  logger?: PluginLogger;
}) {
  let interval: NodeJS.Timeout | null = null;

  return {
    id: "linkclaw-background-sync",
    start: async () => {
      const intervalMs = resolveSyncInterval(params.config.syncIntervalMs);
      const tick = async () => {
        try {
          const envelope = await runLinkClaw(
            params.config,
            {
              command: "message_sync",
            },
            params.pluginRoot,
          );
          const record =
            typeof envelope.result === "object" && envelope.result !== null
              ? (envelope.result as Record<string, unknown>)
              : undefined;
          const synced = typeof record?.synced === "number" ? record.synced : 0;
          if (synced > 0) {
            params.logger?.info?.(`linkclaw: background sync pulled ${synced} new message(s)`);
          }
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          if (
            message.includes("state db not found") ||
            message.includes("self messaging profile not found")
          ) {
            return;
          }
          params.logger?.warn?.(`linkclaw: background sync failed: ${message}`);
        }
      };

      await tick();
      interval = setInterval(() => {
        tick().catch((error) => {
          const message = error instanceof Error ? error.message : String(error);
          params.logger?.warn?.(`linkclaw: background sync failed: ${message}`);
        });
      }, intervalMs);
      interval.unref?.();
    },
    stop: async () => {
      if (interval) {
        clearInterval(interval);
        interval = null;
      }
    },
  };
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

function resolveSyncInterval(value: number | undefined): number {
  if (!Number.isFinite(value) || value === undefined) {
    return DEFAULT_BACKGROUND_SYNC_INTERVAL_MS;
  }
  return Math.max(5000, Math.floor(value));
}
