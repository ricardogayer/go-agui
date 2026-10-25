Wire documents from the .NET SDK's Verify baselines
(sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/baselines/<Sample>/):

  turnNN.request.json  = <Test>#Turn_NN.02.Request.Client.AGUI.verified.json (the RunAgentInput the client sent;
                         identical to .03.Request.Server.AGUI in every baseline)
  turnNN.events.json   = <Test>#Turn_NN.07.Response.Client.AGUI.verified.json with every "rawEvent" member removed
                         (the events as they crossed the wire; rawEvent carries .NET-specific MEAI serializations)

Ids were already scrubbed by the C# test (thread_Id_1, call_Id_1, chatcmpl-Id_1, guid_Id_1, ...).
