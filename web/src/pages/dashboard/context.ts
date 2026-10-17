import { useOutletContext } from 'react-router-dom'
import type { ChildWithSummary, FamilySpace, TokenGetter } from '../../api'

// Everything the dashboard sub-pages need, fetched ONCE by DashboardLayout
// and handed down via router outlet context.
export interface DashboardContext {
  auth: TokenGetter
  family: FamilySpace
  role: string // 'owner' | 'parent' — fail-closed default is 'parent'
  children: ChildWithSummary[]
  // True until the FIRST children fetch resolves — drives the skeletons.
  childrenLoading: boolean
  refreshChildren: () => Promise<void>
  refreshMe: () => Promise<void>
}

export function useDashboard(): DashboardContext {
  return useOutletContext<DashboardContext>()
}
