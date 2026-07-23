import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import { InputGroupAddon } from '@/components/ui/input-group'
import {
  getLaneAgentModels,
  getLaneAgents,
  laneAgentModelsQueryKey,
  laneAgentsQueryKey,
  laneQueryKey,
  patchLane,
  type Lane,
  type LaneAgent,
  type LaneAgentModel,
  type LaneAgentModelSource,
} from '@/lib/api'

type ReasoningOption = {
  id: string
  label: string
}

function formatEffortLabel(effort: string): string {
  if (effort === 'xhigh') return 'Extra high'
  if (effort === 'none') return 'None'
  return effort.charAt(0).toUpperCase() + effort.slice(1)
}

type Props = {
  lane: Lane
  isOwner: boolean
  turnRunning: boolean
}

export function LaneAgentStrip({ lane, isOwner, turnRunning }: Props) {
  const queryClient = useQueryClient()
  const controlsDisabled = !isOwner || turnRunning
  const modelSource = lane.model_source ?? ''

  const agentsQuery = useQuery({
    queryKey: laneAgentsQueryKey,
    queryFn: getLaneAgents,
  })
  const modelsQuery = useQuery({
    queryKey: laneAgentModelsQueryKey(lane.agent_provider, modelSource),
    queryFn: () => getLaneAgentModels(lane.agent_provider, modelSource),
    enabled: Boolean(lane.agent_provider && modelSource),
    retry: 1,
  })

  const availableAgents = useMemo(
    () => (agentsQuery.data?.agents ?? []).filter((agent) => agent.available),
    [agentsQuery.data],
  )
  const models = modelsQuery.data?.models ?? []

  const selectedAgent =
    availableAgents.find((agent) => agent.provider === lane.agent_provider) ??
    null

  const sourceOptions = selectedAgent?.model_sources ?? []
  const showSource = sourceOptions.length > 1
  const selectedSource =
    sourceOptions.find((source) => source.id === modelSource) ?? null

  const selectedModel =
    models.find((model) => model.id === lane.agent_model) ?? null

  const reasoningOptions = useMemo((): ReasoningOption[] => {
    const efforts = selectedModel?.reasoning_efforts ?? []
    return efforts.map((effort) => ({
      id: effort,
      label: formatEffortLabel(effort),
    }))
  }, [selectedModel])

  const selectedReasoning =
    reasoningOptions.find((option) => option.id === lane.reasoning_effort) ??
    null
  const reasoningDisabled =
    controlsDisabled ||
    !selectedModel ||
    reasoningOptions.length === 0 ||
    !selectedModel.supports_reasoning

  const patchMutation = useMutation({
    mutationFn: (payload: Parameters<typeof patchLane>[1]) =>
      patchLane(lane.id, payload),
    onSuccess: async (updated) => {
      queryClient.setQueryData(laneQueryKey(lane.id), updated)
      await queryClient.invalidateQueries({
        queryKey: laneAgentModelsQueryKey(
          updated.agent_provider,
          updated.model_source ?? '',
        ),
      })
    },
  })

  function switchAgent(agent: LaneAgent | null) {
    if (!agent || agent.provider === lane.agent_provider) return
    if (
      !window.confirm(
        'Switching agents starts a new AI session. Farplane will hand off a summary of this chat. Files in the Lane computer stay as they are.',
      )
    ) {
      return
    }
    patchMutation.mutate({ agent_provider: agent.provider })
  }

  function switchSource(source: LaneAgentModelSource | null) {
    if (!source || source.id === modelSource) return
    patchMutation.mutate({ model_source: source.id })
  }

  function switchModel(model: LaneAgentModel | null) {
    if (!model || model.id === lane.agent_model) return
    const nextEffort =
      model.default_reasoning_effort ?? model.reasoning_efforts[0] ?? null
    patchMutation.mutate({
      agent_model: model.id,
      reasoning_effort: nextEffort,
    })
  }

  function switchReasoning(option: ReasoningOption | null) {
    if (!option || option.id === lane.reasoning_effort) return
    patchMutation.mutate({ reasoning_effort: option.id })
  }

  return (
    <div className="flex flex-wrap items-center gap-3">
      <Combobox
        items={availableAgents}
        value={selectedAgent}
        onValueChange={switchAgent}
        itemToStringLabel={(agent: LaneAgent) => agent.label}
        itemToStringValue={(agent: LaneAgent) => agent.provider}
        isItemEqualToValue={(a: LaneAgent, b: LaneAgent) =>
          a.provider === b.provider
        }
        disabled={controlsDisabled || patchMutation.isPending}
      >
        <ComboboxInput
          placeholder="Choose agent"
          className="h-9 min-w-[10rem]"
          disabled={controlsDisabled || patchMutation.isPending}
        >
          <InputGroupAddon
            align="inline-start"
            className="text-xs tracking-[0.02em]"
          >
            Agent
          </InputGroupAddon>
        </ComboboxInput>
        <ComboboxContent>
          <ComboboxEmpty>No available agents</ComboboxEmpty>
          <ComboboxList>
            {(agent: LaneAgent) => (
              <ComboboxItem key={agent.provider} value={agent}>
                {agent.label}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>

      {showSource ? (
        <Combobox
          items={sourceOptions}
          value={selectedSource}
          onValueChange={switchSource}
          itemToStringLabel={(source: LaneAgentModelSource) => source.label}
          itemToStringValue={(source: LaneAgentModelSource) => source.id}
          isItemEqualToValue={(
            a: LaneAgentModelSource,
            b: LaneAgentModelSource,
          ) => a.id === b.id}
          disabled={controlsDisabled || patchMutation.isPending}
        >
          <ComboboxInput
            placeholder="Choose source"
            className="h-9 min-w-[10rem]"
            disabled={controlsDisabled || patchMutation.isPending}
          >
            <InputGroupAddon
              align="inline-start"
              className="text-xs tracking-[0.02em]"
            >
              Source
            </InputGroupAddon>
          </ComboboxInput>
          <ComboboxContent>
            <ComboboxEmpty>No sources</ComboboxEmpty>
            <ComboboxList>
              {(source: LaneAgentModelSource) => (
                <ComboboxItem key={source.id} value={source}>
                  {source.label}
                </ComboboxItem>
              )}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      ) : null}

      <Combobox
        items={models}
        value={selectedModel}
        onValueChange={switchModel}
        itemToStringLabel={(model: LaneAgentModel) => model.label}
        itemToStringValue={(model: LaneAgentModel) => model.id}
        isItemEqualToValue={(a: LaneAgentModel, b: LaneAgentModel) =>
          a.id === b.id
        }
        disabled={
          controlsDisabled ||
          patchMutation.isPending ||
          modelsQuery.isLoading ||
          models.length === 0 ||
          !modelSource
        }
      >
        <ComboboxInput
          placeholder={
            modelsQuery.isError
              ? 'Models unavailable'
              : modelsQuery.isLoading
                ? 'Loading…'
                : 'Choose model'
          }
          className="h-9 min-w-[12rem]"
          disabled={
            controlsDisabled ||
            patchMutation.isPending ||
            modelsQuery.isLoading ||
            models.length === 0 ||
            !modelSource
          }
        >
          <InputGroupAddon
            align="inline-start"
            className="text-xs tracking-[0.02em]"
          >
            Model
          </InputGroupAddon>
        </ComboboxInput>
        <ComboboxContent>
          <ComboboxEmpty>No models</ComboboxEmpty>
          <ComboboxList>
            {(model: LaneAgentModel) => (
              <ComboboxItem key={model.id} value={model}>
                {model.label}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>

      <Combobox
        items={reasoningOptions}
        value={selectedReasoning}
        onValueChange={switchReasoning}
        itemToStringLabel={(option: ReasoningOption) => option.label}
        itemToStringValue={(option: ReasoningOption) => option.id}
        isItemEqualToValue={(a: ReasoningOption, b: ReasoningOption) =>
          a.id === b.id
        }
        disabled={reasoningDisabled || patchMutation.isPending}
      >
        <ComboboxInput
          placeholder={
            reasoningOptions.length === 0 ? 'Not available' : 'Choose effort'
          }
          className="h-9 min-w-[9rem]"
          disabled={reasoningDisabled || patchMutation.isPending}
        >
          <InputGroupAddon
            align="inline-start"
            className="text-xs tracking-[0.02em]"
          >
            Reasoning
          </InputGroupAddon>
        </ComboboxInput>
        <ComboboxContent>
          <ComboboxEmpty>No reasoning options</ComboboxEmpty>
          <ComboboxList>
            {(option: ReasoningOption) => (
              <ComboboxItem key={option.id} value={option}>
                {option.label}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>

      <p className="text-xs text-muted-foreground">
        Shared with everyone in this lane
      </p>

      {patchMutation.isError ? (
        <p className="w-full text-xs text-destructive">
          {patchMutation.error.message}
          <Button
            type="button"
            size="xs"
            variant="ghost"
            className="ml-2"
            onClick={() => patchMutation.reset()}
          >
            Dismiss
          </Button>
        </p>
      ) : null}
    </div>
  )
}
