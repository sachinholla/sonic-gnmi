package gnmi

import (
	"context"
	"encoding/json"
	"errors"
	io "io/ioutil"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	log "github.com/golang/glog"
	gnoi_file_pb "github.com/openconfig/gnoi/file"
	gnoi_os_pb "github.com/openconfig/gnoi/os"
	gnoi_system_pb "github.com/openconfig/gnoi/system"
	"github.com/sonic-net/sonic-gnmi/common_utils"
	spb "github.com/sonic-net/sonic-gnmi/proto/gnoi"
	spb_jwt "github.com/sonic-net/sonic-gnmi/proto/gnoi/jwt"
	ssc "github.com/sonic-net/sonic-gnmi/sonic_service_client"
	transutil "github.com/sonic-net/sonic-gnmi/transl_utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ReadFileStat(path string) (*gnoi_file_pb.StatInfo, error) {
	sc, err := ssc.NewDbusClient()
	if err != nil {
		return nil, err
	}
	defer sc.Close()

	log.V(2).Infof("Reading file stat at path %s...", path)
	data, err := sc.GetFileStat(path)
	if err != nil {
		log.V(2).Infof("Failed to read file stat at path %s: %v. Error ", path, err)
		return nil, err
	}
	// Parse the data and populate StatInfo
	lastModified, err := strconv.ParseUint(data["last_modified"], 10, 64)
	if err != nil {
		return nil, err
	}

	permissions, err := strconv.ParseUint(data["permissions"], 8, 32)
	if err != nil {
		return nil, err
	}

	size, err := strconv.ParseUint(data["size"], 10, 64)
	if err != nil {
		return nil, err
	}

	umaskStr := data["umask"]
	if strings.HasPrefix(umaskStr, "o") {
		umaskStr = umaskStr[1:] // Remove leading "o"
	}
	umask, err := strconv.ParseUint(umaskStr, 8, 32)
	if err != nil {
		return nil, err
	}

	statInfo := &gnoi_file_pb.StatInfo{
		Path:         data["path"],
		LastModified: lastModified,
		Permissions:  uint32(permissions),
		Size:         size,
		Umask:        uint32(umask),
	}
	return statInfo, nil
}

func (srv *FileServer) Stat(ctx context.Context, req *gnoi_file_pb.StatRequest) (*gnoi_file_pb.StatResponse, error) {
	_, err := authenticate(srv.config, ctx, false)
	if err != nil {
		return nil, err
	}
	path := req.GetPath()
	log.V(1).Info("gNOI: Read File Stat")
	log.V(1).Info("Request: ", req)
	statInfo, err := ReadFileStat(path)
	if err != nil {
		return nil, err
	}
	resp := &gnoi_file_pb.StatResponse{
		Stats: []*gnoi_file_pb.StatInfo{statInfo},
	}
	return resp, nil
}

// TODO: Support GNOI File Get
func (srv *FileServer) Get(req *gnoi_file_pb.GetRequest, stream gnoi_file_pb.File_GetServer) error {
	log.V(1).Info("gNOI: File Get")
	return status.Errorf(codes.Unimplemented, "")
}

func KillOrRestartProcess(restart bool, serviceName string) error {
	sc, err := ssc.NewDbusClient()
	if err != nil {
		return err
	}
	defer sc.Close()

	if restart {
		log.V(2).Infof("Restarting service %s...", serviceName)
		err = sc.RestartService(serviceName)
		if err != nil {
			log.V(2).Infof("Failed to restart service %s: %v", serviceName, err)
		}
	} else {
		log.V(2).Infof("Stopping service %s...", serviceName)
		err = sc.StopService(serviceName)
		if err != nil {
			log.V(2).Infof("Failed to stop service %s: %v", serviceName, err)
		}
	}
	return err
}

func (srv *OSServer) Verify(ctx context.Context, req *gnoi_os_pb.VerifyRequest) (*gnoi_os_pb.VerifyResponse, error) {
	_, err := authenticate(srv.config, ctx, false)
	if err != nil {
		log.V(2).Infof("Failed to authenticate: %v", err)
		return nil, err
	}

	log.V(1).Info("gNOI: Verify")
	dbus, err := ssc.NewDbusClient()
	if err != nil {
		log.V(2).Infof("Failed to create dbus client: %v", err)
		return nil, err
	}
	defer dbus.Close()

	image_json, err := dbus.ListImages()
	if err != nil {
		log.V(2).Infof("Failed to list images: %v", err)
		return nil, err
	}

	images := make(map[string]interface{})
	err = json.Unmarshal([]byte(image_json), &images)
	if err != nil {
		log.V(2).Infof("Failed to unmarshal images: %v", err)
		return nil, err
	}

	current, exists := images["current"]
	if !exists {
		return nil, status.Errorf(codes.Internal, "Key 'current' not found in images")
	}
	current_image, ok := current.(string)
	if !ok {
		return nil, status.Errorf(codes.Internal, "Failed to assert current image as string")
	}
	resp := &gnoi_os_pb.VerifyResponse{
		Version: current_image,
	}
	return resp, nil
}

func (srv *OSServer) Activate(ctx context.Context, req *gnoi_os_pb.ActivateRequest) (*gnoi_os_pb.ActivateResponse, error) {
	_, err := authenticate(srv.config, ctx /*writeAccess=*/, true)
	if err != nil {
		log.Errorf("Failed to authenticate: %v", err)
		return nil, err
	}

	log.Infof("gNOI: Activate")
	image := req.GetVersion()
	log.Infof("Requested to activate image %s", image)

	dbus, err := ssc.NewDbusClient()
	if err != nil {
		log.Errorf("Failed to create dbus client: %v", err)
		return nil, err
	}
	defer dbus.Close()

	var resp gnoi_os_pb.ActivateResponse
	err = dbus.ActivateImage(image)
	if err != nil {
		log.Errorf("Failed to activate image %s: %v", image, err)
		image_not_exists := os.IsNotExist(err) ||
			(strings.Contains(strings.ToLower(err.Error()), "not") &&
				strings.Contains(strings.ToLower(err.Error()), "exist"))
		if image_not_exists {
			// Image does not exist.
			resp.Response = &gnoi_os_pb.ActivateResponse_ActivateError{
				ActivateError: &gnoi_os_pb.ActivateError{
					Type:   gnoi_os_pb.ActivateError_NON_EXISTENT_VERSION,
					Detail: err.Error(),
				},
			}
		} else {
			// Other error.
			resp.Response = &gnoi_os_pb.ActivateResponse_ActivateError{
				ActivateError: &gnoi_os_pb.ActivateError{
					Type:   gnoi_os_pb.ActivateError_UNSPECIFIED,
					Detail: err.Error(),
				},
			}
		}
		return &resp, nil
	}

	log.Infof("Successfully activated image %s", image)
	resp.Response = &gnoi_os_pb.ActivateResponse_ActivateOk{}
	return &resp, nil
}

func (srv *SystemServer) KillProcess(ctx context.Context, req *gnoi_system_pb.KillProcessRequest) (*gnoi_system_pb.KillProcessResponse, error) {
	_, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}

	serviceName := req.GetName()
	restart := req.GetRestart()
	if req.GetPid() != 0 {
		return nil, status.Errorf(codes.Unimplemented, "Pid option is not implemented")
	}
	if req.GetSignal() != gnoi_system_pb.KillProcessRequest_SIGNAL_TERM {
		return nil, status.Errorf(codes.Unimplemented, "KillProcess only supports SIGNAL_TERM (option 1) for graceful process termination. Please specify SIGNAL_TERM")
	}
	log.V(1).Info("gNOI: KillProcess with optional restart")
	log.V(1).Info("Request: ", req)
	err = KillOrRestartProcess(restart, serviceName)
	if err != nil {
		return nil, err
	}
	var resp gnoi_system_pb.KillProcessResponse
	return &resp, nil
}

func HaltSystem() error {
	sc, err := ssc.NewDbusClient()
	if err != nil {
		return err
	}
	defer sc.Close()

	log.V(2).Infof("Halting the system..")
	err = sc.HaltSystem()
	if err != nil {
		log.V(2).Infof("Failed to Halt the system %v", err)
	}
	return err
}

func RebootSystem(fileName string) error {
	log.V(2).Infof("Rebooting with %s...", fileName)
	sc, err := ssc.NewDbusClient()
	if err != nil {
		return err
	}
	defer sc.Close()

	err = sc.ConfigReload(fileName)
	return err
}

func (srv *SystemServer) Reboot(ctx context.Context, req *gnoi_system_pb.RebootRequest) (*gnoi_system_pb.RebootResponse, error) {
	fileName := common_utils.GNMI_WORK_PATH + "/config_db.json.tmp"

	_, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Reboot")
	log.V(1).Info("Request:", req)

	// Check the reboot type
	switch req.GetMethod() {
	case gnoi_system_pb.RebootMethod_HALT:
		log.V(1).Info("Reboot method is HALT. Halting the system...")
		err = HaltSystem()
		if err != nil {
			return nil, err
		}
	default:
		log.V(1).Info("Reboot system now, delay is ignored...")
		// TODO: Support GNOI reboot delay
		// Delay in nanoseconds before issuing reboot.
		// https://github.com/openconfig/gnoi/blob/master/system/system.proto#L102-L115
		config_db_json, err := io.ReadFile(fileName)
		if errors.Is(err, os.ErrNotExist) {
			fileName = ""
		}
		err = RebootSystem(string(config_db_json))
		if err != nil {
			return nil, err
		}
	}

	var resp gnoi_system_pb.RebootResponse
	return &resp, nil
}

// TODO: Support GNOI RebootStatus
func (srv *SystemServer) RebootStatus(ctx context.Context, req *gnoi_system_pb.RebootStatusRequest) (*gnoi_system_pb.RebootStatusResponse, error) {
	_, err := authenticate(srv.config, ctx, false)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: RebootStatus")
	return nil, status.Errorf(codes.Unimplemented, "")
}

// TODO: Support GNOI CancelReboot
func (srv *SystemServer) CancelReboot(ctx context.Context, req *gnoi_system_pb.CancelRebootRequest) (*gnoi_system_pb.CancelRebootResponse, error) {
	_, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: CancelReboot")
	return nil, status.Errorf(codes.Unimplemented, "")
}
func (srv *SystemServer) Ping(req *gnoi_system_pb.PingRequest, rs gnoi_system_pb.System_PingServer) error {
	ctx := rs.Context()
	_, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return err
	}
	log.V(1).Info("gNOI: Ping")
	return status.Errorf(codes.Unimplemented, "")
}
func (srv *SystemServer) Traceroute(req *gnoi_system_pb.TracerouteRequest, rs gnoi_system_pb.System_TracerouteServer) error {
	ctx := rs.Context()
	_, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return err
	}
	log.V(1).Info("gNOI: Traceroute")
	return status.Errorf(codes.Unimplemented, "")
}
func (srv *SystemServer) SetPackage(rs gnoi_system_pb.System_SetPackageServer) error {
	ctx := rs.Context()
	_, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return err
	}
	log.V(1).Info("gNOI: SetPackage")
	return status.Errorf(codes.Unimplemented, "")
}
func (srv *SystemServer) SwitchControlProcessor(ctx context.Context, req *gnoi_system_pb.SwitchControlProcessorRequest) (*gnoi_system_pb.SwitchControlProcessorResponse, error) {
	_, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: SwitchControlProcessor")
	return nil, status.Errorf(codes.Unimplemented, "")
}
func (srv *SystemServer) Time(ctx context.Context, req *gnoi_system_pb.TimeRequest) (*gnoi_system_pb.TimeResponse, error) {
	_, err := authenticate(srv.config, ctx, false)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Time")
	var tm gnoi_system_pb.TimeResponse
	tm.Time = uint64(time.Now().UnixNano())
	return &tm, nil
}

func (srv *Server) Authenticate(ctx context.Context, req *spb_jwt.AuthenticateRequest) (*spb_jwt.AuthenticateResponse, error) {
	// Can't enforce normal authentication here.. maybe only enforce client cert auth if enabled?
	// ctx,err := authenticate(srv.config, ctx, false)
	// if err != nil {
	// 	return nil, err
	// }
	log.V(1).Info("gNOI: Sonic Authenticate")

	if !srv.config.UserAuth.Enabled("jwt") {
		return nil, status.Errorf(codes.Unimplemented, "")
	}
	auth_success, _ := UserPwAuth(req.Username, req.Password)
	if auth_success {
		usr, err := user.Lookup(req.Username)
		if err == nil {
			roles, err := GetUserRoles(usr)
			if err == nil {
				return &spb_jwt.AuthenticateResponse{Token: tokenResp(req.Username, roles)}, nil
			}
		}

	}
	return nil, status.Errorf(codes.PermissionDenied, "Invalid Username or Password")

}
func (srv *Server) Refresh(ctx context.Context, req *spb_jwt.RefreshRequest) (*spb_jwt.RefreshResponse, error) {
	ctx, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Sonic Refresh")

	if !srv.config.UserAuth.Enabled("jwt") {
		return nil, status.Errorf(codes.Unimplemented, "")
	}

	token, _, err := JwtAuthenAndAuthor(ctx)
	if err != nil {
		return nil, err
	}

	claims := &Claims{}
	jwt.ParseWithClaims(token.AccessToken, claims, func(token *jwt.Token) (interface{}, error) {
		return hmacSampleSecret, nil
	})
	if time.Unix(claims.ExpiresAt, 0).Sub(time.Now()) > JwtRefreshInt {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid JWT Token")
	}

	return &spb_jwt.RefreshResponse{Token: tokenResp(claims.Username, claims.Roles)}, nil

}

func (srv *Server) ClearNeighbors(ctx context.Context, req *spb.ClearNeighborsRequest) (*spb.ClearNeighborsResponse, error) {
	ctx, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Sonic ClearNeighbors")
	log.V(1).Info("Request: ", req)

	resp := &spb.ClearNeighborsResponse{
		Output: &spb.ClearNeighborsResponse_Output{},
	}

	reqstr, err := json.Marshal(req)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	jsresp, err := transutil.TranslProcessAction("/sonic-neighbor:clear-neighbors", []byte(reqstr), ctx)

	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	err = json.Unmarshal(jsresp, resp)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	return resp, nil
}

func (srv *Server) CopyConfig(ctx context.Context, req *spb.CopyConfigRequest) (*spb.CopyConfigResponse, error) {
	ctx, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Sonic CopyConfig")

	resp := &spb.CopyConfigResponse{
		Output: &spb.SonicOutput{},
	}

	reqstr, err := json.Marshal(req)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}
	jsresp, err := transutil.TranslProcessAction("/sonic-config-mgmt:copy", []byte(reqstr), ctx)

	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	err = json.Unmarshal(jsresp, resp)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	return resp, nil
}

func (srv *Server) ShowTechsupport(ctx context.Context, req *spb.TechsupportRequest) (*spb.TechsupportResponse, error) {
	ctx, err := authenticate(srv.config, ctx, false)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Sonic ShowTechsupport")

	resp := &spb.TechsupportResponse{
		Output: &spb.TechsupportResponse_Output{},
	}

	reqstr, err := json.Marshal(req)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}
	jsresp, err := transutil.TranslProcessAction("/sonic-show-techsupport:sonic-show-techsupport-info", []byte(reqstr), ctx)

	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	err = json.Unmarshal(jsresp, resp)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	return resp, nil
}

func (srv *Server) ImageInstall(ctx context.Context, req *spb.ImageInstallRequest) (*spb.ImageInstallResponse, error) {
	ctx, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Sonic ImageInstall")

	resp := &spb.ImageInstallResponse{
		Output: &spb.SonicOutput{},
	}

	reqstr, err := json.Marshal(req)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}
	jsresp, err := transutil.TranslProcessAction("/sonic-image-management:image-install", []byte(reqstr), ctx)

	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	err = json.Unmarshal(jsresp, resp)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	return resp, nil
}

func (srv *Server) ImageRemove(ctx context.Context, req *spb.ImageRemoveRequest) (*spb.ImageRemoveResponse, error) {
	ctx, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Sonic ImageRemove")

	resp := &spb.ImageRemoveResponse{
		Output: &spb.SonicOutput{},
	}

	reqstr, err := json.Marshal(req)

	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}
	jsresp, err := transutil.TranslProcessAction("/sonic-image-management:image-remove", []byte(reqstr), ctx)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	err = json.Unmarshal(jsresp, resp)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}
	return resp, nil
}

func (srv *Server) ImageDefault(ctx context.Context, req *spb.ImageDefaultRequest) (*spb.ImageDefaultResponse, error) {
	ctx, err := authenticate(srv.config, ctx, true)
	if err != nil {
		return nil, err
	}
	log.V(1).Info("gNOI: Sonic ImageDefault")

	resp := &spb.ImageDefaultResponse{
		Output: &spb.SonicOutput{},
	}

	reqstr, err := json.Marshal(req)

	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}
	jsresp, err := transutil.TranslProcessAction("/sonic-image-management:image-default", []byte(reqstr), ctx)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	err = json.Unmarshal(jsresp, resp)
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}

	return resp, nil
}
