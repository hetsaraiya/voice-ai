import React, { FC, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useGlobalNavigation } from '@/hooks/use-global-navigator';
import { toHumanReadableDateTime } from '@/utils/date';
import { Add, Renew, ChartLine, Edit, TrashCan } from '@carbon/icons-react';
import { useCurrentCredential } from '@/hooks/use-credential';
import { useRapidaStore } from '@/hooks';
import { SectionLoader } from '@/app/components/loader/section-loader';
import toast from 'react-hot-toast/headless';
import { EmptyState } from '@/app/components/carbon/empty-state';
import { CreateAssistantAnalysis } from '@/app/pages/assistant/actions/configure-assistant-analysis/create-assistant-analysis';
import { useAssistantAnalysisPageStore } from '@/app/pages/assistant/actions/store/use-analysis-page-store';
import { UpdateAssistantAnalysis } from '@/app/pages/assistant/actions/configure-assistant-analysis/update-assistant-analysis';
import { IconOnlyButton, PrimaryButton } from '@/app/components/carbon/button';
import { Pagination } from '@/app/components/carbon/pagination';
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  TableBatchActions,
  TableBatchAction,
  RadioButton,
} from '@carbon/react';
import { TableSection } from '@/app/components/sections/table-section';

const getAnalysisOptionMap = (row: any): Map<string, string> => {
  const map = new Map<string, string>();
  const options = row?.getOptionsList?.() || [];
  options.forEach((option: any) => {
    const key = option?.getKey?.();
    const value = option?.getValue?.();
    if (key && typeof value === 'string') {
      map.set(key, value);
    }
  });
  return map;
};

const getAnalysisEndpointId = (row: any): string =>
  getAnalysisOptionMap(row).get('endpoint_id') || '';

const getAnalysisEndpointVersion = (row: any): string =>
  getAnalysisOptionMap(row).get('endpoint_version') || 'latest';

export function ConfigureAssistantAnalysisPage() {
  const { assistantId } = useParams();
  return (
    <>
      {assistantId && <ConfigureAssistantAnalysis assistantId={assistantId} />}
    </>
  );
}

export function CreateAssistantAnalysisPage() {
  const { assistantId } = useParams();
  return (
    <>{assistantId && <CreateAssistantAnalysis assistantId={assistantId} />}</>
  );
}

export function UpdateAssistantAnalysisPage() {
  const { assistantId } = useParams();
  return (
    <>{assistantId && <UpdateAssistantAnalysis assistantId={assistantId} />}</>
  );
}

const ConfigureAssistantAnalysis: FC<{ assistantId: string }> = ({
  assistantId,
}) => {
  const navigation = useGlobalNavigation();
  const axtion = useAssistantAnalysisPageStore();
  const { authId, token, projectId } = useCurrentCredential();
  const { loading, showLoader, hideLoader } = useRapidaStore();
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedAnalysisId, setSelectedAnalysisId] = useState<string | null>(
    null,
  );

  const get = () => {
    showLoader('block');
    axtion.getAssistantAnalysis(
      assistantId,
      projectId,
      token,
      authId,
      e => {
        toast.error(e);
        hideLoader();
      },
      () => {
        hideLoader();
      },
    );
  };

  useEffect(() => {
    get();
  }, []);

  const deleteAssistantAnalysis = (assistantId: string, analysisId: string) => {
    showLoader('block');
    axtion.deleteAssistantAnalysis(
      assistantId,
      analysisId,
      projectId,
      token,
      authId,
      e => {
        toast.error(e);
        hideLoader();
      },
      () => {
        get();
      },
    );
  };

  const filteredAnalyses = searchTerm.trim()
    ? axtion.analysises.filter(row =>
        [
          row.getName(),
          getAnalysisEndpointId(row),
          getAnalysisEndpointVersion(row),
          row.getExecutionpriority(),
          row.getStatus(),
        ]
          .join(' ')
          .toLowerCase()
          .includes(searchTerm.trim().toLowerCase()),
      )
    : axtion.analysises;

  const selectedAnalysis = filteredAnalyses.find(
    row => row.getId() === selectedAnalysisId,
  );

  if (loading) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center">
        <SectionLoader />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col flex-1">
      <div className="px-4 pt-4 pb-6 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
        <div>
          <Breadcrumb noTrailingSlash className="mb-2">
            <BreadcrumbItem
              href={`/deployment/assistant/${assistantId}/overview`}
            >
              Assistant
            </BreadcrumbItem>
          </Breadcrumb>
          <h1 className="text-2xl font-light tracking-tight">Analysis</h1>
        </div>
      </div>
      <TableToolbar>
        <TableBatchActions
          shouldShowBatchActions={!!selectedAnalysis}
          totalSelected={selectedAnalysis ? 1 : 0}
          totalCount={filteredAnalyses.length}
          onCancel={() => setSelectedAnalysisId(null)}
        >
          {selectedAnalysis && (
            <>
              <TableBatchAction
                renderIcon={Edit}
                
                onClick={() => {
                  navigation.goToEditAssistantAnalysis(
                    assistantId,
                    selectedAnalysis.getId(),
                  );
                  setSelectedAnalysisId(null);
                }}
              >
                Edit analysis
              </TableBatchAction>
              <TableBatchAction
                renderIcon={TrashCan}
                
                onClick={() => {
                  deleteAssistantAnalysis(
                    assistantId,
                    selectedAnalysis.getId(),
                  );
                  setSelectedAnalysisId(null);
                }}
              >
                Delete analysis
              </TableBatchAction>
            </>
          )}
        </TableBatchActions>
        <TableToolbarContent>
          <TableToolbarSearch
            placeholder="Search analysis..."
            onChange={(e: any) => setSearchTerm(e.target?.value || '')}
          />
          <IconOnlyButton
            kind="ghost"
            size="lg"
            renderIcon={Renew}
            iconDescription="Refresh"
            onClick={get}
          />
          <PrimaryButton
            size="md"
            renderIcon={Add}
            onClick={() => navigation.goToCreateAssistantAnalysis(assistantId)}
          >
            Create analysis
          </PrimaryButton>
        </TableToolbarContent>
      </TableToolbar>
      <TableSection>
        {axtion.analysises.length > 0 && filteredAnalyses.length > 0 ? (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader className="!w-12" />
                  <TableHeader>Name</TableHeader>
                  <TableHeader>Endpoint</TableHeader>
                  <TableHeader>Version</TableHeader>
                  <TableHeader>Priority</TableHeader>
                  <TableHeader>Created</TableHeader>
                  <TableHeader>Action</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {filteredAnalyses.map(row => {
                  const selected = selectedAnalysisId === row.getId();
                  return (
                    <TableRow
                      key={row.getId()}
                      isSelected={selected}
                      onClick={() =>
                        setSelectedAnalysisId(selected ? null : row.getId())
                      }
                      className="cursor-pointer"
                    >
                      <TableCell
                        className="!w-12 !pr-0"
                        onClick={e => e.stopPropagation()}
                      >
                        <RadioButton
                          id={`analysis-select-${row.getId()}`}
                          name="analysis-select"
                          labelText=""
                          hideLabel
                          checked={selected}
                          onChange={() =>
                            setSelectedAnalysisId(selected ? null : row.getId())
                          }
                        />
                      </TableCell>
                      <TableCell>{row.getName()}</TableCell>
                      <TableCell>
                        <span className="font-mono text-xs">
                          {getAnalysisEndpointId(row) || '—'}
                        </span>
                      </TableCell>
                      <TableCell>{getAnalysisEndpointVersion(row)}</TableCell>
                      <TableCell>{row.getExecutionpriority()}</TableCell>
                      <TableCell>
                        {row.getCreateddate()
                          ? toHumanReadableDateTime(row.getCreateddate()!)
                          : '—'}
                      </TableCell>
                      <TableCell onClick={e => e.stopPropagation()}>
                        <div className="flex items-center gap-0">
                          <Button
                            hasIconOnly
                            renderIcon={Edit}
                            iconDescription="Edit analysis"
                            kind="ghost"
                            size="sm"
                            onClick={() =>
                              navigation.goToEditAssistantAnalysis(
                                assistantId,
                                row.getId(),
                              )
                            }
                          />
                          <Button
                            hasIconOnly
                            renderIcon={TrashCan}
                            iconDescription="Delete analysis"
                            kind="danger--ghost"
                            size="sm"
                            onClick={() =>
                              deleteAssistantAnalysis(assistantId, row.getId())
                            }
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            <Pagination
              totalItems={axtion.totalCount}
              page={axtion.page}
              pageSize={axtion.pageSize}
              pageSizes={[10, 20, 50]}
              onChange={({ page, pageSize }) => {
                if (pageSize !== axtion.pageSize) {
                  axtion.setPageSize(pageSize);
                } else {
                  axtion.setPage(page);
                }
              }}
            />
          </>
        ) : axtion.analysises.length > 0 ? (
          <EmptyState
            icon={ChartLine}
            title="No analysis found"
            subtitle="No analysis matched your search."
          />
        ) : (
          <EmptyState
            icon={ChartLine}
            title="No analysis"
            subtitle="Any analysis you add will be listed here."
          />
        )}
      </TableSection>
    </div>
  );
};
