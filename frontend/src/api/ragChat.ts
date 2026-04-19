import { buildApiUrl } from './runtime';
import { request, getErrorMessage } from './request';

// ========== 绫诲瀷瀹氫箟 ==========

export interface RagChatSession {
  id: number;
  title: string;
  knowledgeBaseIds: number[];
  createdAt: string;
}

export interface RagChatSessionListItem {
  id: number;
  title: string;
  messageCount: number;
  knowledgeBaseNames: string[];
  updatedAt: string;
  isPinned: boolean;
}

export interface RagChatMessage {
  id: number;
  type: 'user' | 'assistant';
  content: string;
  createdAt: string;
}

export interface KnowledgeBaseItem {
  id: number;
  name: string;
  originalFilename: string;
  fileSize: number;
  contentType: string;
  uploadedAt: string;
  lastAccessedAt: string;
  accessCount: number;
  questionCount: number;
}

export interface RagChatSessionDetail {
  id: number;
  title: string;
  knowledgeBases: KnowledgeBaseItem[];
  messages: RagChatMessage[];
  createdAt: string;
  updatedAt: string;
}

// ========== API 鍑芥暟 ==========

export const ragChatApi = {
  /**
   * 鍒涘缓鏂颁細璇?
   */
  async createSession(knowledgeBaseIds: number[], title?: string): Promise<RagChatSession> {
    return request.post<RagChatSession>('/api/rag-chat/sessions', {
      knowledgeBaseIds,
      title,
    });
  },

  /**
   * 鑾峰彇浼氳瘽鍒楄〃
   */
  async listSessions(): Promise<RagChatSessionListItem[]> {
    return request.get<RagChatSessionListItem[]>('/api/rag-chat/sessions');
  },

  /**
   * 鑾峰彇浼氳瘽璇︽儏
   */
  async getSessionDetail(sessionId: number): Promise<RagChatSessionDetail> {
    return request.get<RagChatSessionDetail>(`/api/rag-chat/sessions/${sessionId}`);
  },

  /**
   * 鏇存柊浼氳瘽鏍囬
   */
  async updateSessionTitle(sessionId: number, title: string): Promise<void> {
    return request.put(`/api/rag-chat/sessions/${sessionId}/title`, { title });
  },

  /**
   * 鏇存柊浼氳瘽鐭ヨ瘑搴?
   */
  async updateKnowledgeBases(sessionId: number, knowledgeBaseIds: number[]): Promise<void> {
    return request.put(`/api/rag-chat/sessions/${sessionId}/knowledge-bases`, {
      knowledgeBaseIds,
    });
  },

  /**
   * 鍒囨崲浼氳瘽缃《鐘舵€?
   */
  async togglePin(sessionId: number): Promise<void> {
    return request.put(`/api/rag-chat/sessions/${sessionId}/pin`);
  },

  /**
   * 鍒犻櫎浼氳瘽
   */
  async deleteSession(sessionId: number): Promise<void> {
    return request.delete(`/api/rag-chat/sessions/${sessionId}`);
  },

  /**
   * 鍙戦€佹秷鎭紙娴佸紡SSE锛?
   */
  async sendMessageStream(
    sessionId: number,
    question: string,
    onMessage: (chunk: string) => void,
    onComplete: () => void,
    onError: (error: Error) => void
  ): Promise<void> {
    try {
      const response = await fetch(
        buildApiUrl(`/api/rag-chat/sessions/${sessionId}/messages/stream`),
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ question }),
        }
      );

      if (!response.ok) {
        // 灏濊瘯瑙ｆ瀽閿欒鍝嶅簲
        try {
          const errorData = await response.json();
          if (errorData && errorData.message) {
            throw new Error(errorData.message);
          }
        } catch {
          // 蹇界暐瑙ｆ瀽閿欒
        }
        throw new Error(`璇锋眰澶辫触 (${response.status})`);
      }

      const reader = response.body?.getReader();
      if (!reader) {
        throw new Error('Unable to read response stream');
      }

      const decoder = new TextDecoder();
      let buffer = '';

      // 浠?SSE 浜嬩欢涓彁鍙栧唴瀹?
      const extractEventContent = (event: string): string | null => {
        if (!event.trim()) return null;

        const lines = event.split('\n');
        const contentParts: string[] = [];

        for (const line of lines) {
          if (line.startsWith('data:')) {
            // 鎻愬彇 data: 鍚庨潰鐨勫唴瀹癸紝淇濈暀鍘熷鏍煎紡锛堝寘鎷缉杩涚┖鏍硷級
            // ServerSentEvent 涓嶄細鍦?data: 鍚庢坊鍔犻澶栫┖鏍?
            contentParts.push(line.substring(5));
          }
        }

        if (contentParts.length === 0) return null;

        // 鍚堝苟鍐呭骞惰繕鍘熻浆涔夌殑鎹㈣绗?
        return contentParts.join('')
          .replace(/\\n/g, '\n')
          .replace(/\\r/g, '\r');
      };

      while (true) {
        const { done, value } = await reader.read();

        if (done) {
          if (buffer) {
            const content = extractEventContent(buffer);
            if (content) {
              onMessage(content);
            }
          }
          onComplete();
          break;
        }

        buffer += decoder.decode(value, { stream: true });

        // SSE 浜嬩欢浠?\n\n 鍒嗛殧锛屼絾涔熼渶瑕佸鐞嗗崟琛岀殑鎯呭喌
        const newlineIndex = buffer.indexOf('\n\n');
        if (newlineIndex === -1) {
          // 濡傛灉娌℃湁鎵惧埌 \n\n锛屽皾璇曞鐞嗗崟琛?data: 鏍煎紡
          const singleLineIndex = buffer.indexOf('\n');
          if (singleLineIndex !== -1 && buffer.substring(0, singleLineIndex).startsWith('data:')) {
            const line = buffer.substring(0, singleLineIndex);
            const content = extractEventContent(line);
            if (content) {
              onMessage(content);
            }
            buffer = buffer.substring(singleLineIndex + 1);
          }
          continue;
        }

        // 澶勭悊瀹屾暣鐨勪簨浠跺潡
        const eventBlock = buffer.substring(0, newlineIndex);
        buffer = buffer.substring(newlineIndex + 2);

        const content = extractEventContent(eventBlock);
        if (content !== null) {
          onMessage(content);
        }
      }
    } catch (error) {
      onError(new Error(getErrorMessage(error)));
    }
  },
};




