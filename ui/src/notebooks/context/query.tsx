import React, {FC, useContext, useMemo} from 'react'
import {connect} from 'react-redux'
import {AppState, Variable, Organization} from 'src/types'
import {runQuery} from 'src/shared/apis/query'
import {getWindowVars} from 'src/variables/utils/getWindowVars'
import {buildVarsOption} from 'src/variables/utils/buildVarsOption'
import {getTimeRangeVars} from 'src/variables/utils/getTimeRangeVars'
import {getVariables, asAssignment} from 'src/variables/selectors'
import {getOrg} from 'src/organizations/selectors'
import {NotebookContext} from 'src/notebooks/context/notebook'
import {TimeContext} from 'src/notebooks/context/time'
import {fromFlux as parse} from '@influxdata/giraffe'
import {BothResults} from 'src/notebooks'
import {reportSimpleQueryPerformanceEvent} from 'src/cloud/utils/reporting'

export interface QueryContextType {
  query: (text: string) => Promise<BothResults>
}

export const DEFAULT_CONTEXT: QueryContextType = {
  query: () => Promise.resolve({} as BothResults),
}

export const QueryContext = React.createContext<QueryContextType>(
  DEFAULT_CONTEXT
)

type Props = StateProps
export const QueryProvider: FC<Props> = ({children, variables, org}) => {
  const {id} = useContext(NotebookContext)
  const {timeContext} = useContext(TimeContext)
  const time = timeContext[id]

  const vars = useMemo(
    () =>
      variables.map(v => asAssignment(v)).concat(getTimeRangeVars(time.range)),
    [variables, time]
  )
  const query = (text: string) => {
    const windowVars = getWindowVars(text, vars)
    const extern = buildVarsOption([...vars, ...windowVars])

    reportSimpleQueryPerformanceEvent('runQuery', {context: 'notebooks'})
    return runQuery(org.id, text, extern)
      .promise.then(raw => {
        if (raw.type !== 'SUCCESS') {
          throw new Error(raw.message)
        }

        return raw
      })
      .then(raw => {
        return {
          source: text,
          raw: raw.csv,
          parsed: parse(raw.csv),
          error: null,
        }
      })
  }

  if (!time) {
    return null
  }

  return (
    <QueryContext.Provider value={{query}}>{children}</QueryContext.Provider>
  )
}

interface StateProps {
  variables: Variable[]
  org: Organization
}

const mstp = (state: AppState): StateProps => {
  const variables = getVariables(state)
  const org = getOrg(state)

  return {
    org,
    variables,
  }
}

const ConnectedQueryProvider = connect<StateProps>(mstp)(QueryProvider)

export default ConnectedQueryProvider
