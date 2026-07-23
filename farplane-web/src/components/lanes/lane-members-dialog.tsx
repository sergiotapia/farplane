import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ApiError,
  addLaneParticipant,
  createLaneInvite,
  getActiveLaneInvite,
  getLaneParticipants,
  getOrganizationMembers,
  laneActiveInviteQueryKey,
  laneParticipantsQueryKey,
  lanesQueryKey,
  organizationMembersQueryKey,
  regenerateLaneInvite,
  removeLaneParticipant,
  revokeActiveLaneInvite,
  type MeUser,
} from '@/lib/api'

type Props = {
  laneId: string
  open: boolean
  currentUser: MeUser
  onOpenChange: (open: boolean) => void
}

export function LaneMembersDialog({
  laneId,
  open,
  currentUser,
  onOpenChange,
}: Props) {
  const queryClient = useQueryClient()
  const [addMember, setAddMember] = useState<{
    id: string
    display_name: string
    email: string
  } | null>(null)
  const [membersError, setMembersError] = useState<string | null>(null)

  const participantsQuery = useQuery({
    queryKey: laneParticipantsQueryKey(laneId),
    queryFn: () => getLaneParticipants(laneId),
    enabled: open,
  })
  const membersQuery = useQuery({
    queryKey: organizationMembersQueryKey,
    queryFn: getOrganizationMembers,
    enabled: open,
  })
  const activeInviteQuery = useQuery({
    queryKey: laneActiveInviteQueryKey(laneId),
    queryFn: () => getActiveLaneInvite(laneId),
    enabled: open,
    retry: false,
  })

  const participants = participantsQuery.data?.participants ?? []
  const seatedIds = useMemo(
    () => new Set(participants.map((participant) => participant.user_id)),
    [participants],
  )
  const addableMembers = useMemo(
    () =>
      (membersQuery.data?.members ?? []).filter(
        (member) => !seatedIds.has(member.id),
      ),
    [membersQuery.data, seatedIds],
  )

  async function refreshAfterPeopleChange() {
    await queryClient.invalidateQueries({
      queryKey: laneParticipantsQueryKey(laneId),
    })
    await queryClient.invalidateQueries({ queryKey: lanesQueryKey })
  }

  const addMutation = useMutation({
    mutationFn: (userId: string) => addLaneParticipant(laneId, userId),
    onSuccess: async () => {
      setAddMember(null)
      setMembersError(null)
      await refreshAfterPeopleChange()
    },
    onError: (err) => {
      setMembersError(
        err instanceof ApiError ? err.message : 'Could not add participant',
      )
    },
  })

  const removeMutation = useMutation({
    mutationFn: (userId: string) => removeLaneParticipant(laneId, userId),
    onSuccess: async () => {
      await refreshAfterPeopleChange()
    },
  })

  const ensureInviteMutation = useMutation({
    mutationFn: () => createLaneInvite(laneId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: laneActiveInviteQueryKey(laneId),
      })
    },
  })

  const regenerateInviteMutation = useMutation({
    mutationFn: () => regenerateLaneInvite(laneId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: laneActiveInviteQueryKey(laneId),
      })
    },
  })

  const revokeInviteMutation = useMutation({
    mutationFn: () => revokeActiveLaneInvite(laneId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: laneActiveInviteQueryKey(laneId),
      })
    },
  })

  const activeInvite =
    activeInviteQuery.isSuccess ? activeInviteQuery.data : null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Members</DialogTitle>
          <DialogDescription>
            Add Organization members immediately, or share an open Lane Invite
            link.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6">
          <div className="space-y-2">
            <h3 className="text-sm font-medium">Participants</h3>
            <ul className="space-y-2 text-sm">
              {participants.map((participant) => (
                <li
                  key={participant.id}
                  className="flex items-center justify-between gap-2"
                >
                  <span className="min-w-0 truncate">
                    {participant.display_name ||
                      participant.email ||
                      participant.user_id.slice(0, 8)}
                    {participant.role === 'owner' ? ' · owner' : ''}
                  </span>
                  {participant.role !== 'owner' &&
                  participant.user_id !== currentUser.id ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={removeMutation.isPending}
                      onClick={() =>
                        removeMutation.mutate(participant.user_id)
                      }
                    >
                      Remove
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          </div>

          <div className="space-y-2">
            <Label>Add Organization member</Label>
            <Combobox
              items={addableMembers}
              value={addMember}
              onValueChange={setAddMember}
              itemToStringLabel={(member) =>
                `${member.display_name} (${member.email})`
              }
              itemToStringValue={(member) => member.id}
              isItemEqualToValue={(a, b) => a.id === b.id}
            >
              <ComboboxInput
                placeholder="Select a member"
                className="w-full"
              />
              <ComboboxContent>
                <ComboboxEmpty>No members to add</ComboboxEmpty>
                <ComboboxList>
                  {(member) => (
                    <ComboboxItem key={member.id} value={member}>
                      {member.display_name} ({member.email})
                    </ComboboxItem>
                  )}
                </ComboboxList>
              </ComboboxContent>
            </Combobox>
            <Button
              type="button"
              size="sm"
              disabled={!addMember || addMutation.isPending}
              onClick={() => {
                if (!addMember) return
                addMutation.mutate(addMember.id)
              }}
            >
              {addMutation.isPending ? 'Adding…' : 'Add participant'}
            </Button>
          </div>

          <div className="space-y-2">
            <h3 className="text-sm font-medium">Lane Invite link</h3>
            {activeInvite ? (
              <div className="space-y-2">
                <Input readOnly value={activeInvite.accept_url} />
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      void navigator.clipboard.writeText(
                        activeInvite.accept_url,
                      )
                    }
                  >
                    Copy URL
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={regenerateInviteMutation.isPending}
                    onClick={() => {
                      if (
                        !window.confirm(
                          'Regenerate the invite link? The old link stops working.',
                        )
                      ) {
                        return
                      }
                      regenerateInviteMutation.mutate()
                    }}
                  >
                    Regenerate
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    disabled={revokeInviteMutation.isPending}
                    onClick={() => revokeInviteMutation.mutate()}
                  >
                    Revoke
                  </Button>
                </div>
              </div>
            ) : (
              <Button
                type="button"
                size="sm"
                disabled={
                  ensureInviteMutation.isPending || activeInviteQuery.isLoading
                }
                onClick={() => ensureInviteMutation.mutate()}
              >
                {ensureInviteMutation.isPending
                  ? 'Creating…'
                  : 'Create invite link'}
              </Button>
            )}
          </div>

          {membersError ? (
            <p className="text-destructive text-sm">{membersError}</p>
          ) : null}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
