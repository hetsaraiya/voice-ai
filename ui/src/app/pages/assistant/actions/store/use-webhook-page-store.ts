import { create } from 'zustand';
import { initialPaginated } from '@/types/types.paginated';
import {
  AssistantWebhook,
  Criteria,
  DeleteAssistantWebhookRequest,
  GetAllAssistantWebhookRequest,
  Paginate,
} from '@rapidaai/react';
import {
  AssistantWebhookProperty,
  AssistantWebhookType,
} from './types/types.assistant-webhook';
import {
  DeleteAssistantWebhook,
  GetAllAssistantWebhook,
} from '@rapidaai/react';
import { connectionConfig } from '@/configs';

const initialAssistantWebhook: AssistantWebhookProperty = {
  webhooks: [],
};

/**
 *
 */
export const useAssistantWebhookPageStore = create<AssistantWebhookType>(
  (set, get) => ({
    ...initialAssistantWebhook,
    ...initialPaginated,

    /**
     *
     * @param number
     * @returns
     */
    setPageSize: (pageSize: number) => {
      // when someone change pagesize change the page to zero
      set({
        page: 1,
        pageSize: pageSize,
      });
    },

    /**
     *
     * @param number
     * @returns
     */
    setPage: (pg: number) => {
      set({
        page: pg,
      });
    },

    /**
     *
     * @param number
     * @returns
     */
    setTotalCount: (tc: number) => {
      set({
        totalCount: tc,
      });
    },

    /**
     *
     * @param ep
     */
    onChangeAssistantWebhooks: (ep: AssistantWebhook[]) => {
      set({
        webhooks: ep,
      });
    },

    /**
     *
     * @param k
     * @param v
     */
    addCriteria: (k: string, v: string, logic: string) => {
      let current = get().criteria.filter(
        x => !(x.key === k && x.logic === logic),
      );
      if (v) current.push({ key: k, value: v, logic: logic });
      set({
        criteria: current,
      });
    },

    /**
     *
     * @param v
     */
    addCriterias: (v: { k: string; v: string; logic: string }[]) => {
      let current = get().criteria.filter(
        x => !v.find(y => y.k === x.key && x.logic === y.logic),
      );
      v.forEach(c => {
        current.push({ key: c.k, value: c.v, logic: c.logic });
      });
      set({
        criteria: current,
      });
    },

    /**
     *
     * @param projectId
     * @param token
     * @param userId
     */
    getAssistantWebhook: async (
      assistantId: string,
      projectId: string,
      token: string,
      userId: string,
      onError: (err: string) => void,
      onSuccess: (e: AssistantWebhook[]) => void,
    ) => {
      const req = new GetAllAssistantWebhookRequest();
      req.setAssistantid(assistantId);

      const paginate = new Paginate();
      paginate.setPage(get().page);
      paginate.setPagesize(get().pageSize);
      req.setPaginate(paginate);

      get().criteria.forEach(({ key, value, logic }) => {
        const ctr = new Criteria();
        ctr.setKey(key);
        ctr.setValue(value);
        ctr.setLogic(logic);
        req.addCriterias(ctr);
      });

      try {
        const gur = await GetAllAssistantWebhook(connectionConfig, req, {
          authorization: token,
          'x-project-id': projectId,
          'x-auth-id': userId,
        });

        if (gur?.getSuccess()) {
          get().onChangeAssistantWebhooks(gur.getDataList());
          let paginated = gur.getPaginated();
          if (paginated) {
            get().setTotalCount(paginated.getTotalitem());
          }
          onSuccess(gur.getDataList());
        } else {
          let errorMessage = gur?.getError();
          if (errorMessage) {
            onError(errorMessage.getHumanmessage());
            return;
          }
          onError('Unable to get webhooks, please try again later.');
        }
      } catch {
        onError('Unable to get webhooks, please try again later.');
      }
    },

    /**
     *
     * @param assistantId
     * @param webhookId
     * @param projectId
     * @param token
     * @param userId
     * @param onError
     * @param onSuccess
     */
    deleteAssistantWebhook: async (
      assistantId: string,
      webhookId: string,
      projectId: string,
      token: string,
      userId: string,
      onError: (err: string) => void,
      onSuccess: (e: AssistantWebhook) => void,
    ) => {
      const req = new DeleteAssistantWebhookRequest();
      req.setAssistantid(assistantId);
      req.setId(webhookId);

      try {
        const gur = await DeleteAssistantWebhook(connectionConfig, req, {
          authorization: token,
          'x-project-id': projectId,
          'x-auth-id': userId,
        });

        if (gur?.getSuccess() && gur.getData()) {
          onSuccess(gur.getData()!);
        } else {
          let errorMessage = gur?.getError();
          if (errorMessage) {
            onError(errorMessage.getHumanmessage());
            return;
          }
          onError(
            'Unable to delete assistant webhook, please try again later.',
          );
        }
      } catch {
        onError('Unable to delete assistant webhook, please try again later.');
      }
    },
    /**
     * columns
     */
    columns: [
      { name: 'ID', key: 'id', visible: false },
      { name: 'HTTP Endpoint', key: 'httpUrl', visible: true },
      { name: 'Events', key: 'events', visible: true },
      { name: 'Max Retry Count', key: 'maxRetryCount', visible: false },
      { name: 'Timeout Seconds', key: 'timeoutSeconds', visible: false },
      { name: 'ExecutionPriority', key: 'executionPriority', visible: true },
      { name: 'Status', key: 'status', visible: true },
      { name: 'Created Date', key: 'created_date', visible: true },
    ],

    /**
     *
     * @param cl
     */
    setColumns(cl: { name: string; key: string; visible: boolean }[]) {
      set({
        columns: cl,
      });
    },

    /**
     *
     * @param k
     * @returns
     */
    visibleColumn: (k: string): boolean => {
      const column = get().columns.find(c => c.key === k);
      return column ? column.visible : false;
    },

    /**
     * clear everything from the context
     * @returns
     */
    clear: () => set({ ...initialAssistantWebhook, ...initialPaginated }),
  }),
);
