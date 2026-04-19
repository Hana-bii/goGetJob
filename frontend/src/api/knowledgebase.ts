import axios from 'axios';
import { buildApiUrl } from './runtime';
import { getErrorMessage, request } from './request';

// 鍚戦噺鍖栫姸鎬?
export type VectorStatus = 'PENDING' | 'PROCESSING' | 'COMPLETED' | 'FAILED';

export interface KnowledgeBaseItem {
  id: number;
  name: string;
  category: string | null;
  originalFilename: string;
  fileSize: number;
  contentType: string;
  uploadedAt: string;
  lastAccessedAt: string;
  accessCount: number;
  questionCount: number;
  vectorStatus: VectorStatus;
  vectorError: string | null;
  chunkCount: number;
}

// 缁熻淇℃伅
export interface KnowledgeBaseStats {
  totalCount: number;
  totalQuestionCount: number;
  totalAccessCount: number;
  completedCount: number;
  processingCount: number;
}

export type SortOption = 'time' | 'size' | 'access' | 'question';

export interface UploadKnowledgeBaseResponse {
  knowledgeBase: {
    id: number;
    name: string;
    category: string;
    fileSize: number;
    contentLength: number;
  };
  storage: {
    fileKey: string;
    fileUrl: string;
  };
  duplicate: boolean;
}

export interface QueryRequest {
  knowledgeBaseIds: number[];  // 鏀寔澶氫釜鐭ヨ瘑搴?
  question: string;
}

export interface QueryResponse {
  answer: string;
  knowledgeBaseId: number;
  knowledgeBaseName: string;
}

export const knowledgeBaseApi = {
  /**
   * 涓婁紶鐭ヨ瘑搴撴枃浠?
   */
  async uploadKnowledgeBase(file: File, name?: string, category?: string): Promise<UploadKnowledgeBaseResponse> {
    const formData = new FormData();
    formData.append('file', file);
    if (name) {
      formData.append('name', name);
    }
    if (category) {
      formData.append('category', category);
    }
    return request.upload<UploadKnowledgeBaseResponse>('/api/knowledgebase/upload', formData);
  },

    /**
     * 涓嬭浇鐭ヨ瘑搴撴枃浠?
     */
    async downloadKnowledgeBase(id: number): Promise<Blob> {
        const response = await axios.get(buildApiUrl(`/api/knowledgebase/${id}/download`), {
            responseType: 'blob',
        });
        return response.data;
    },

  /**
   * 鑾峰彇鎵€鏈夌煡璇嗗簱鍒楄〃
   */
  async getAllKnowledgeBases(sortBy?: SortOption, vectorStatus?: 'PENDING' | 'PROCESSING' | 'COMPLETED' | 'FAILED'): Promise<KnowledgeBaseItem[]> {
    const params = new URLSearchParams();
    if (sortBy) {
      params.append('sortBy', sortBy);
    }
    if (vectorStatus) {
      params.append('vectorStatus', vectorStatus);
    }
    const queryString = params.toString();
    return request.get<KnowledgeBaseItem[]>(`/api/knowledgebase/list${queryString ? `?${queryString}` : ''}`);
  },

  /**
   * 鑾峰彇鐭ヨ瘑搴撹鎯?
   */
  async getKnowledgeBase(id: number): Promise<KnowledgeBaseItem> {
    return request.get<KnowledgeBaseItem>(`/api/knowledgebase/${id}`);
  },

  /**
   * 鍒犻櫎鐭ヨ瘑搴?
   */
  async deleteKnowledgeBase(id: number): Promise<void> {
    return request.delete(`/api/knowledgebase/${id}`);
  },

  // ========== 鍒嗙被绠＄悊 ==========

  /**
   * 鑾峰彇鎵€鏈夊垎绫?
   */
  async getAllCategories(): Promise<string[]> {
    return request.get<string[]>('/api/knowledgebase/categories');
  },

  /**
   * 鏍规嵁鍒嗙被鑾峰彇鐭ヨ瘑搴?
   */
  async getByCategory(category: string): Promise<KnowledgeBaseItem[]> {
    return request.get<KnowledgeBaseItem[]>(`/api/knowledgebase/category/${encodeURIComponent(category)}`);
  },

  /**
   * 鑾峰彇鏈垎绫荤殑鐭ヨ瘑搴?
   */
  async getUncategorized(): Promise<KnowledgeBaseItem[]> {
    return request.get<KnowledgeBaseItem[]>('/api/knowledgebase/uncategorized');
  },

  /**
   * 鏇存柊鐭ヨ瘑搴撳垎绫?
   */
  async updateCategory(id: number, category: string | null): Promise<void> {
    return request.put(`/api/knowledgebase/${id}/category`, { category });
  },

  // ========== 鎼滅储 ==========

  /**
   * 鎼滅储鐭ヨ瘑搴?
   */
  async search(keyword: string): Promise<KnowledgeBaseItem[]> {
    return request.get<KnowledgeBaseItem[]>(`/api/knowledgebase/search?keyword=${encodeURIComponent(keyword)}`);
  },

  // ========== 缁熻 ==========

  /**
   * 鑾峰彇鐭ヨ瘑搴撶粺璁′俊鎭?
   */
  async getStatistics(): Promise<KnowledgeBaseStats> {
    return request.get<KnowledgeBaseStats>('/api/knowledgebase/stats');
  },

  // ========== 鍚戦噺鍖栫鐞?==========

  /**
   * 閲嶆柊鍚戦噺鍖栫煡璇嗗簱锛堟墜鍔ㄩ噸璇曪級
   */
  async revectorize(id: number): Promise<void> {
    return request.post(`/api/knowledgebase/${id}/revectorize`);
  },

  /**
   * 鍩轰簬鐭ヨ瘑搴撳洖绛旈棶棰?
   */
  async queryKnowledgeBase(req: QueryRequest): Promise<QueryResponse> {
    return request.post<QueryResponse>('/api/knowledgebase/query', req, {
      timeout: 180000, // 3鍒嗛挓瓒呮椂
    });
  },

  /**
   * 鍩轰簬鐭ヨ瘑搴撳洖绛旈棶棰橈紙娴佸紡SSE锛?
   * 娉ㄦ剰锛歋SE 浣跨敤 fetch API锛屼笉璧扮粺涓€鐨?axios 灏佽
   */
  async queryKnowledgeBaseStream(
    req: QueryRequest,
    onMessage: (chunk: string) => void,
    onComplete: () => void,
    onError: (error: Error) => void
  ): Promise<void> {
    try {
      const response = await fetch(buildApiUrl('/api/knowledgebase/query/stream'), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(req),
      });

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

      // 杈呭姪鍑芥暟锛氬鐞?data: 琛屽苟鎻愬彇鍐呭
      const extractContent = (line: string): string | null => {
        if (!line.startsWith('data:')) {
          return null;
        }
        let content = line.substring(5); // 绉婚櫎 "data:" 鍓嶇紑
        // SSE 鏍囧噯锛氬鏋?data: 鍚庣涓€涓瓧绗︽槸绌烘牸锛岃繖鏄崗璁眰闈㈢殑绌烘牸锛屽簲璇ョЩ闄?
        // 浣嗚繖鏄彲閫夌殑锛屾湁浜涘疄鐜板彲鑳芥病鏈夎繖涓┖鏍?
        if (content.startsWith(' ')) {
          content = content.substring(1);
        }
        // 濡傛灉鍐呭涓虹┖锛坉ata: 鎴?data: 锛夛紝鍙兘琛ㄧず鎹㈣锛岃繑鍥炴崲琛岀
        if (content.length === 0) {
          return '\n';
        }
        return content;
      };

      while (true) {
        const { done, value } = await reader.read();

        if (done) {
          // 澶勭悊鍓╀綑鐨?buffer
          if (buffer) {
            const content = extractContent(buffer);
            if (content) {
              onMessage(content);
            }
          }
          onComplete();
          break;
        }

        // 瑙ｇ爜鏁版嵁鍧楀苟娣诲姞鍒?buffer
        buffer += decoder.decode(value, { stream: true });

        // 鎸夎鍒嗗壊澶勭悊 SSE 鏍煎紡
        // SSE 鏍煎紡锛歞ata: content\n 鎴?data:content\n锛岀┖琛?\n\n 琛ㄧず浜嬩欢缁撴潫
        const lines = buffer.split('\n');
        // 淇濈暀鏈€鍚庝竴琛岋紙鍙兘涓嶅畬鏁达紝绛夊緟鏇村鏁版嵁锛?
        buffer = lines.pop() || '';

        // 澶勭悊瀹屾暣鐨勮
        for (const line of lines) {
          const content = extractContent(line);
          if (content !== null) {
            // 鍙戦€佸唴瀹癸紙淇濈暀鎵€鏈夋牸寮忥紝鍖呮嫭绌烘牸銆佹崲琛岀瓑锛屽洜涓?Markdown 闇€瑕侊級
            onMessage(content);
          }
          // 绌鸿锛坙ine === ''锛夊湪 SSE 涓〃绀轰簨浠剁粨鏉燂紝浣嗘垜浠笉闇€瑕佺壒娈婂鐞?
          // 鍥犱负姣忎釜 data: 琛屽凡缁忔槸涓€涓畬鏁寸殑鏁版嵁鍧?
        }
      }
    } catch (error) {
      onError(new Error(getErrorMessage(error)));
    }
  },
};



