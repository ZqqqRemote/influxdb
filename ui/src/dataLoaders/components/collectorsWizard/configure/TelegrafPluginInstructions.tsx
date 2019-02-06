// Libraries
import React, {PureComponent, ChangeEvent} from 'react'
import {connect} from 'react-redux'

// Components
import {Form, Input, InputType, ComponentSize} from 'src/clockface'
import FancyScrollbar from 'src/shared/components/fancy_scrollbar/FancyScrollbar'
import OnboardingButtons from 'src/onboarding/components/OnboardingButtons'
import PluginsSideBar from 'src/dataLoaders/components/collectorsWizard/configure/PluginsSideBar'

// Actions
import {
  setTelegrafConfigName,
  setActiveTelegrafPlugin,
  setPluginConfiguration,
} from 'src/dataLoaders/actions/dataLoaders'
import {
  incrementCurrentStepIndex,
  decrementCurrentStepIndex,
} from 'src/dataLoaders/actions/steps'

// Types
import {AppState} from 'src/types/v2/index'
import {TelegrafPlugin} from 'src/types/v2/dataLoaders'

interface DispatchProps {
  onSetTelegrafConfigName: typeof setTelegrafConfigName
  onSetActiveTelegrafPlugin: typeof setActiveTelegrafPlugin
  onSetPluginConfiguration: typeof setPluginConfiguration
  onIncrementStep: typeof incrementCurrentStepIndex
  onDecrementStep: typeof decrementCurrentStepIndex
}

interface StateProps {
  telegrafConfigName: string
  telegrafPlugins: TelegrafPlugin[]
}

type Props = DispatchProps & StateProps

export class TelegrafPluginInstructions extends PureComponent<Props> {
  public render() {
    const {
      telegrafConfigName,
      telegrafPlugins,
      onDecrementStep,
      onIncrementStep,
    } = this.props
    return (
      <Form onSubmit={onIncrementStep}>
        <div className="wizard-step--scroll-area">
          <div className="wizard--columns">
            <PluginsSideBar
              telegrafPlugins={telegrafPlugins}
              onTabClick={this.handleClickSideBarTab}
              title="Plugins to Configure"
              visible={this.sideBarVisible}
            />
            <FancyScrollbar autoHide={false}>
              <div className="wizard-step--scroll-content">
                <h3 className="wizard-step--title">
                  Telegraf Configuration Information
                </h3>
                <h5 className="wizard-step--sub-title">
                  Telegraf is a plugin based data collection agent. Click on the
                  plugin names to the left in order to configure the selected
                  plugins. For more information about Telegraf Plugins, see
                  documentation.
                </h5>
                <Form.Element label="Telegraf Configuration Name">
                  <Input
                    type={InputType.Text}
                    value={telegrafConfigName}
                    onChange={this.handleNameInput}
                    titleText="Telegraf Configuration Name"
                    size={ComponentSize.Medium}
                    autoFocus={true}
                  />
                </Form.Element>
              </div>
            </FancyScrollbar>
          </div>
        </div>

        <OnboardingButtons
          onClickBack={onDecrementStep}
          nextButtonText={'Create and Verify'}
        />
      </Form>
    )
  }

  private get sideBarVisible() {
    const {telegrafPlugins} = this.props

    return telegrafPlugins.length > 0
  }

  private handleNameInput = (e: ChangeEvent<HTMLInputElement>) => {
    this.props.onSetTelegrafConfigName(e.target.value)
  }

  private handleClickSideBarTab = (tabID: string) => {
    const {
      onSetActiveTelegrafPlugin,
      telegrafPlugins,
      onSetPluginConfiguration,
    } = this.props

    const activeTelegrafPlugin = telegrafPlugins.find(tp => tp.active)
    if (!!activeTelegrafPlugin) {
      onSetPluginConfiguration(activeTelegrafPlugin.name)
    }

    onSetActiveTelegrafPlugin(tabID)
  }
}

const mstp = ({
  dataLoading: {
    dataLoaders: {telegrafConfigName, telegrafPlugins},
  },
}: AppState): StateProps => {
  return {
    telegrafConfigName,
    telegrafPlugins,
  }
}

const mdtp: DispatchProps = {
  onSetTelegrafConfigName: setTelegrafConfigName,
  onIncrementStep: incrementCurrentStepIndex,
  onDecrementStep: decrementCurrentStepIndex,
  onSetActiveTelegrafPlugin: setActiveTelegrafPlugin,
  onSetPluginConfiguration: setPluginConfiguration,
}

export default connect<StateProps, DispatchProps, {}>(
  mstp,
  mdtp
)(TelegrafPluginInstructions)
