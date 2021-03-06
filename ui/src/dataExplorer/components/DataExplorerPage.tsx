// Libraries
import React, {FC} from 'react'

// Components
import DataExplorer from 'src/dataExplorer/components/DataExplorer'
import {Page} from '@influxdata/clockface'
import SaveAsButton from 'src/dataExplorer/components/SaveAsButton'
import VisOptionsButton from 'src/timeMachine/components/VisOptionsButton'
import ViewTypeDropdown from 'src/timeMachine/components/view_options/ViewTypeDropdown'
import GetResources from 'src/resources/components/GetResources'
import TimeZoneDropdown from 'src/shared/components/TimeZoneDropdown'
import DeleteDataButton from 'src/dataExplorer/components/DeleteDataButton'
import CloudUpgradeButton from 'src/shared/components/CloudUpgradeButton'

// Utils
import {pageTitleSuffixer} from 'src/shared/utils/pageTitles'
import {useLoadTimeReporting} from 'src/cloud/utils/reporting'

// Types
import {ResourceType} from 'src/types'

const DataExplorerPage: FC = ({children}) => {
  useLoadTimeReporting('DataExplorerPage load start')

  return (
    <Page titleTag={pageTitleSuffixer(['Data Explorer'])}>
      {children}
      <GetResources resources={[ResourceType.Variables]}>
        <Page.Header fullWidth={true}>
          <Page.Title title="Data Explorer" />
          <CloudUpgradeButton />
        </Page.Header>
        <Page.ControlBar fullWidth={true}>
          <Page.ControlBarLeft>
            <ViewTypeDropdown />
            <VisOptionsButton />
          </Page.ControlBarLeft>
          <Page.ControlBarRight>
            <DeleteDataButton />
            <TimeZoneDropdown />
            <SaveAsButton />
          </Page.ControlBarRight>
        </Page.ControlBar>
        <Page.Contents fullWidth={true} scrollable={false}>
          <DataExplorer />
        </Page.Contents>
      </GetResources>
    </Page>
  )
}

export default DataExplorerPage
